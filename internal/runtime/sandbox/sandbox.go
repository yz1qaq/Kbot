// Package sandbox 使用 Docker 一次性容器隔离执行 Python 和 Shell 代码。
package sandbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Config 定义代码执行的资源与隔离配置。
type Config struct {
	PythonImage    string        // 执行 Python 使用的镜像
	BashImage      string        // 执行 Shell 代码使用的镜像
	NetworkMode    string        // Docker 网络模式，例如 none
	MemoryMB       int           // 容器内存限制
	CPUs           float64       // CPU 配额，例如 0.5
	PidsLimit      int           // 进程、线程数量限制
	Timeout        time.Duration // 单次执行的超时时间
	TmpfsMB        int           // 临时内存文件系统大小
	MaxCodeBytes   int           // 输入代码的字节数上限
	MaxOutputBytes int           // 输出捕获的大小限制
	MaxConcurrent  int           // 并发执行上限
}

type Result struct {
	ExecutionID     string `json:"execution_id"`     // 本次执行的唯一标识
	Stdout          string `json:"stdout"`           // 标准输出
	Stderr          string `json:"stderr"`           // 标准错误输出
	ExitCode        int    `json:"exit_code"`        // 进程退出码
	DurationMS      int64  `json:"duration_ms"`      // 执行耗时，单位毫秒
	TimedOut        bool   `json:"timed_out"`        // 是否执行超时
	OutputTruncated bool   `json:"output_truncated"` // 捕获的输出是否被截断
}

type Sandbox struct {
	cfg        Config        //保存镜像、资源限制、超时等配置
	dockerPath string        //保存 Docker 命令的可执行文件路径
	semaphore  chan struct{} //控制这个 Sandbox 实例的并发执行数量
}

func DefaultConfig() Config {
	return Config{
		PythonImage:    "python:3.12.11-slim-bookworm@sha256:519591d6871b7bc437060736b9f7456b8731f1499a57e22e6c285135ae657bf7",
		BashImage:      "busybox:1.37.0@sha256:9db7b59979c38555a39def84a31fb98b5296952f9e3afd4f6f11f05b07adfab0",
		NetworkMode:    "none",
		MemoryMB:       256,
		CPUs:           0.5,
		PidsLimit:      64,
		Timeout:        30 * time.Second,
		TmpfsMB:        64,
		MaxCodeBytes:   64 << 10,
		MaxOutputBytes: 1 << 20,
		MaxConcurrent:  4,
	}
}

func normalizeConfig(cfg Config) Config {
	defaults := DefaultConfig()

	if cfg.PythonImage == "" {
		cfg.PythonImage = defaults.PythonImage
	}
	if cfg.BashImage == "" {
		cfg.BashImage = defaults.BashImage
	}
	if cfg.NetworkMode == "" {
		cfg.NetworkMode = defaults.NetworkMode
	}
	if cfg.MemoryMB <= 0 {
		cfg.MemoryMB = defaults.MemoryMB
	}
	if cfg.CPUs <= 0 {
		cfg.CPUs = defaults.CPUs
	}
	if cfg.PidsLimit <= 0 {
		cfg.PidsLimit = defaults.PidsLimit
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaults.Timeout
	}
	if cfg.TmpfsMB <= 0 {
		cfg.TmpfsMB = defaults.TmpfsMB
	}
	if cfg.MaxCodeBytes <= 0 {
		cfg.MaxCodeBytes = defaults.MaxCodeBytes
	}
	if cfg.MaxOutputBytes <= 0 {
		cfg.MaxOutputBytes = defaults.MaxOutputBytes
	}
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = defaults.MaxConcurrent
	}

	return cfg
}

func New(cfg Config) *Sandbox {
	cfg = normalizeConfig(cfg)
	path, _ := exec.LookPath("docker")

	return &Sandbox{
		cfg:        cfg,
		dockerPath: path,
		semaphore:  make(chan struct{}, cfg.MaxConcurrent),
	}
}

func (s *Sandbox) Available() bool {
	return s.dockerPath != ""
}

func (s *Sandbox) dockerInfo(ctx context.Context) error {
	output, err := exec.CommandContext(ctx, s.dockerPath, "info", "--format", "{{.ServerVersion}}").CombinedOutput()
	if err != nil {
		return fmt.Errorf("sandbox:docker daemon unavailable: %v: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

var ErrDockerUnavailable = errors.New("sandbox: docker is unavailable")

func (s *Sandbox) Check(ctx context.Context) error {
	if !s.Available() {
		return ErrDockerUnavailable
	}

	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := s.dockerInfo(checkCtx); err != nil {
		return err
	}

	for _, image := range []string{s.cfg.PythonImage, s.cfg.BashImage} {
		output, err := exec.CommandContext(
			checkCtx,
			s.dockerPath,
			"image",
			"inspect",
			image,
		).CombinedOutput()
		if err != nil {
			return fmt.Errorf(
				"sandbox: image %s is not ready: %w: %s",
				image,
				err,
				strings.TrimSpace(string(output)),
			)
		}
	}

	return nil
}

const sandboxLabel = "kbot.course.sandbox=true"

func (s *Sandbox) Prepare(ctx context.Context) error {
	if !s.Available() {
		return ErrDockerUnavailable
	}
	if err := s.dockerInfo(ctx); err != nil {
		return err
	}

	//查找带sandboxLabel的标签容器
	output, err := exec.CommandContext(ctx, s.dockerPath, "ps", "-aq", "--filter", "label="+sandboxLabel).Output()
	if err != nil {
		return fmt.Errorf("sandbox:List orphan containers: %w", err)
	}
	//强制删除这些容器
	if ids := strings.Fields(string(output)); len(ids) > 0 {
		args := append([]string{"rm", "-f"}, ids...)
		cleanup, err := exec.CommandContext(ctx, s.dockerPath, args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("sandbox:remove orphan containers:%w %s", err, strings.TrimSpace(string(cleanup)))
		}
	}

	seen := make(map[string]struct{}, 2)
	for _, image := range []string{s.cfg.PythonImage, s.cfg.BashImage} {
		if _, ok := seen[image]; ok {
			continue
		}
		seen[image] = struct{}{}

		output, err := exec.CommandContext(
			ctx, s.dockerPath, "pull", image,
		).CombinedOutput()
		if err != nil {
			return fmt.Errorf(
				"sandbox: pull image %s: %w: %s",
				image, err, strings.TrimSpace(string(output)),
			)
		}
	}

	return s.Check(ctx)

}

func (s *Sandbox) buildArgs(language string) ([]string, error) {
	args := []string{
		"run", "--rm", "-i",
		"--label", sandboxLabel,
		"--network", s.cfg.NetworkMode,
		"--read-only",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=" + strconv.Itoa(s.cfg.TmpfsMB) + "m",
		"--memory", strconv.Itoa(s.cfg.MemoryMB) + "m",
		"--cpus", strconv.FormatFloat(s.cfg.CPUs, 'f', -1, 64),
		"--pids-limit", strconv.Itoa(s.cfg.PidsLimit),
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--user", "65534:65534",
		"--workdir", "/tmp",
		"--env", "HOME=/tmp",
		"--env", "PYTHONDONTWRITEBYTECODE=1",
		"--ulimit", "nofile=128:128",
	}

	switch language {
	case "python", "code_run_python":
		args = append(
			args,
			s.cfg.PythonImage,
			"python",
			"-c",
			"import sys; exec(sys.stdin.read())",
		)

	case "bash", "sh", "code_run_bash":
		args = append(
			args,
			s.cfg.BashImage,
			"sh",
			"-s",
		)

	default:
		return nil, fmt.Errorf(
			"sandbox: unsupported language %q",
			language,
		)
	}

	return args, nil
}

// 为每次代码执行生成一个标识，后续用于记录结果和命名容器。
func newExecutionID() string {
	var raw [8]byte

	if _, err := rand.Read(raw[:]); err == nil {
		return hex.EncodeToString(raw[:])
	}

	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

type limitedBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func newLimitedBuffer(limit int) limitedBuffer {
	return limitedBuffer{limit: limit}
}

// 只保存规定大小的输出，超出的部分丢弃。
func (b *limitedBuffer) Write(p []byte) (int, error) {
	originalLength := len(p)
	remaining := b.limit - b.Len()

	if remaining <= 0 {
		b.truncated = true
		return originalLength, nil
	}

	if len(p) > remaining {
		p = p[:remaining]
		b.truncated = true
	}

	_, err := io.Copy(&b.Buffer, bytes.NewReader(p))
	return originalLength, err
}

var ErrCapacity = errors.New("sandbox: runner is at capacity")

func (s *Sandbox) Execute(ctx context.Context, language, code string) (Result, error) {
	started := time.Now()
	result := Result{
		ExecutionID: newExecutionID(),
		ExitCode:    -1,
	}

	if !s.Available() {
		return result, ErrDockerUnavailable
	}
	if code == "" {
		return result, errors.New("sandbox: code is required")
	}
	if len(code) > s.cfg.MaxCodeBytes {
		return result, fmt.Errorf(
			"sandbox: code exceeds %d bytes",
			s.cfg.MaxCodeBytes,
		)
	}

	args, err := s.buildArgs(language)
	if err != nil {
		return result, err
	}

	select {
	case <-ctx.Done():
		return result, ctx.Err()
	default:
	}

	select {
	case s.semaphore <- struct{}{}:
		defer func() { <-s.semaphore }()
	default:
		return result, ErrCapacity
	}

	runCtx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	containerName := "kbot-sandbox-" + result.ExecutionID
	args = append(
		args[:1],
		append([]string{"--name", containerName}, args[1:]...)...,
	)

	command := exec.CommandContext(runCtx, s.dockerPath, args...)
	command.Stdin = strings.NewReader(code)

	stdout := newLimitedBuffer(s.cfg.MaxOutputBytes)
	stderr := newLimitedBuffer(s.cfg.MaxOutputBytes)
	command.Stdout = &stdout
	command.Stderr = &stderr

	err = command.Run()

	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	result.OutputTruncated = stdout.truncated || stderr.truncated
	result.DurationMS = time.Since(started).Milliseconds()

	if exitError, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitError.ExitCode()
	} else if err == nil {
		result.ExitCode = 0
	}

	if runCtx.Err() != nil {
		result.TimedOut = runCtx.Err() == context.DeadlineExceeded

		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cleanupCancel()

		_ = exec.CommandContext(
			cleanupCtx,
			s.dockerPath,
			"rm",
			"-f",
			containerName,
		).Run()

		if result.TimedOut {
			return result, fmt.Errorf(
				"sandbox: execution timed out after %s",
				s.cfg.Timeout,
			)
		}

		return result, fmt.Errorf(
			"sandbox: execution canceled: %w",
			runCtx.Err(),
		)
	}

	if err != nil {
		return result, fmt.Errorf(
			"sandbox: execution failed: %v: %s",
			err,
			strings.TrimSpace(result.Stderr),
		)
	}

	return result, nil
}

func (s *Sandbox) Run(ctx context.Context, language, code string) (string, error) {
	result, err := s.Execute(ctx, language, code)
	return result.Stdout, err
}
