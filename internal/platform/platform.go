// Package platform 是 Agent 控制面。
package platform

// Platform 是控制面服务的装配入口。
// 具体服务会在后续课程中逐步加入。
type Platform struct{}

// New 创建控制面。
func New() *Platform {
	return &Platform{}
}