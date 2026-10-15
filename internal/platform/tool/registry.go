// Package tool 管理可被 Agent 固定引用的工具版本。
package tool

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

type Version struct {
	// 全局唯一的工具版本 ID。
	// 例如：search-v1、search-v2，分别代表同一个工具的两个版本。
	ID string `json:"id"`

	// 工具本身的 ID，用来把多个版本归到同一个工具下。
	// 例如：search-v1 和 search-v2 的 ToolID 都是 search-tool。
	ToolID string `json:"tool_id"`

	// 这个工具版本属于哪个工作空间。
	// 例如：workspace-001。
	WorkspaceID string `json:"workspace_id"`

	// 工具名称；注册时用于检查同一工作空间内的名称冲突。
	// 例如：search_inventory。
	Name string `json:"name"`

	// 工具的来源类型。
	// 例如：HTTP 工具可以用 http 表示。
	SourceType string `json:"source_type"`

	// 工具的功能描述，帮助理解它能做什么。
	// 例如：根据 SKU 查询各仓库的库存。
	Description string `json:"description"`

	// 工具输入参数的 JSON Schema，以字节切片保存。
	// 例如：{"type":"object","properties":{"sku":{"type":"string"}}}
	// json:"-" 表示直接将 Version 编码成 JSON 时，不输出这个字段。
	InputSchema []byte `json:"-"`

	// 调用工具时使用的接口地址。
	// 例如：https://example.com/api/inventory。
	Endpoint string `json:"-"`

	// 明文认证配置。
	// 例如：demo-api-key。
	// Register 会加密它，并清空内部保存副本中的这个字段。
	AuthConfig string `json:"-"`

	// 是否配置了认证信息。
	// Register 根据加密结果设置它，让客户端知道有没有配置凭据。
	HasAuth bool `json:"has_auth"`

	// 是否属于敏感操作。
	// 例如：实际执行库存调拨的工具可以标记为 true。
	// 当前只是一个标记，本身不会自动触发审批。
	Sensitive bool `json:"sensitive"`

	// 是否已经发布。
	// false 表示草稿；后续 Resolve 只允许取出已发布的版本。
	Published bool `json:"published"`

	// 创建时间，需要由调用方赋值。
	// 例如：创建工具版本时设置为 time.Now()。
	CreatedAt time.Time `json:"created_at"`

	// 加密后的认证配置，布局为：[nonce][密文及认证标签]。
	// 小写字段，仅 tool 包内部可以直接访问，也不会被 JSON 编码输出。
	authCiphertext []byte
}

type Registry struct {
	mu sync.RWMutex
	//按版本 ID 保存工具
	versions       map[string]Version
	toolNameOwners map[string]map[string]string
	aead           cipher.AEAD //后续加密凭据使用
}

func NewRegistry(credentialKeys ...[]byte) *Registry {
	key := make([]byte, 32)
	if len(credentialKeys) > 0 && len(credentialKeys[0]) > 0 {
		digest := sha256.Sum256(credentialKeys[0]) //如果传了密钥,就生成固定长度的密钥
		copy(key, digest[:])
	} else if _, err := io.ReadFull(rand.Reader, key); err != nil { ////否则生成随机密钥
		panic(fmt.Errorf("generate credential key: %w", err))
	}

	//创建AES加密器
	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	return &Registry{
		versions:       make(map[string]Version),
		toolNameOwners: make(map[string]map[string]string),
		aead:           aead,
	}
}

// 用于加密工具的认证配置,接受字符串,返回加密后的字节
func (r *Registry) encrypt(plaintext string) ([]byte, error) {
	if plaintext == "" {
		return nil, nil
	}

	//先生成随机的nonce,同一个密钥下,每次加密需要使用不同的nonce,解密需要使用
	nonce := make([]byte, r.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	//[ nonce ][ 密文及认证标签 ]
	return r.aead.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// 用于把保存的认证配置还原为明文
func (r *Registry) decrypt(ciphertext []byte) (string, error) {
	if len(ciphertext) == 0 {
		return "", nil
	}

	nonceSize := r.aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("invalid credential ciphertext")
	}

	plaintext, err := r.aead.Open(
		nil,
		ciphertext[:nonceSize],
		ciphertext[nonceSize:],
		nil,
	)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func (r *Registry) Register(_ context.Context, version Version) error {
	if strings.TrimSpace(version.ID) == "" || strings.TrimSpace(version.WorkspaceID) == "" || strings.TrimSpace(version.Name) == "" {
		return fmt.Errorf("id,workspace and name are required")
	}

	//如果ToolID为空,那么就让ToolID=ID
	if strings.TrimSpace(version.ToolID) == "" {
		version.ToolID = version.ID
	}

	var schema map[string]any
	if err := json.Unmarshal(version.InputSchema, &schema); err != nil || schema["type"] != "object" {
		return fmt.Errorf("input schema must be a JSON object schema")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	//判断这个ID是否已经存在
	if _, exist := r.versions[version.ID]; exist {
		return fmt.Errorf("tool version %s already exists", version.ID)
	}

	workspaceNames := r.toolNameOwners[version.WorkspaceID]
	//如果这个空间ID对应的这张工具表不存在,就先创建
	if workspaceNames == nil {
		workspaceNames = make(map[string]string)
		r.toolNameOwners[version.WorkspaceID] = workspaceNames
	}

	/*
		工作空间 workspace-1
		├─ ToolID github-search-tool，Name search_github
		│  ├─ ID github-search-v1
		│  └─ ID github-search-v2
		│
		└─ 工具 github-issue-tool，名称 create_github_issue
		   └─ 版本 github-issue-v1
	*/

	nameKey := strings.ToLower(strings.TrimSpace(version.Name))
	/*
			判断这个工具名字是否已经被使用,并且owner必须和toolID相等
			r.toolNameOwners = map[string]map[string]string{
		    "workspace-1": {
		        Name:ToolID
		        "transfer": "transfer-tool",
		    },
			记录每个工作空间中的工具名归哪个 ToolID 所有
	*/
	if owner, exists := workspaceNames[nameKey]; exists && owner != version.ToolID {
		return fmt.Errorf("tool name %q already exists in workspace", version.Name)
	}

	ciphertext, err := r.encrypt(version.AuthConfig)
	if err != nil {
		return fmt.Errorf("encrypt tool credentials: %w", err)
	}

	version.HasAuth = len(ciphertext) > 0
	version.AuthConfig = ""
	version.authCiphertext = ciphertext
	version.InputSchema = append([]byte(nil), version.InputSchema...)

	r.versions[version.ID] = version
	workspaceNames[nameKey] = version.ToolID
	return nil
}

// 根据版本ID和工作空间取出发布的工具版本
func (r *Registry) Resolve(_ context.Context, workspaceID, versionID string) (Version, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	//判断是否版本已经存在
	version, ok := r.versions[versionID]
	if !ok || version.WorkspaceID != workspaceID {
		return Version{}, fmt.Errorf("tool version %s not found", versionID)
	}

	//是否允许上线
	if !version.Published {
		return Version{}, fmt.Errorf("tool version %s is not published", versionID)
	}

	version.InputSchema = append([]byte(nil), version.InputSchema...)

	plaintext, err := r.decrypt(version.authCiphertext)
	if err != nil {
		return Version{}, fmt.Errorf("decrypt tool credentials: %w", err)
	}

	version.AuthConfig = plaintext
	version.authCiphertext = nil
	return version, nil
}

func (r *Registry) List(_ context.Context, workspaceID string) []Version {
	r.mu.RLock()
	defer r.mu.RUnlock()

	versions := make([]Version, 0, len(r.versions))
	for _, version := range r.versions {
		if version.WorkspaceID != workspaceID {
			continue
		}

		version.InputSchema = append([]byte(nil), version.InputSchema...)
		version.AuthConfig = ""
		version.authCiphertext = nil
		versions = append(versions, version)
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].ID < versions[j].ID
	})
	return versions
}
