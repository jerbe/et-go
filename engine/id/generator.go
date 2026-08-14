package id

import "sync/atomic"

// Generator 基于 Snowflake 思想的简单 ID 生成器。
// 对应 ET 框架的 InstanceId 生成。
type Generator struct {
	counter atomic.Int64
}

// NewGenerator 创建 ID 生成器
func NewGenerator() *Generator {
	return &Generator{}
}

// Next 生成下一个唯一 ID
func (g *Generator) Next() int64 {
	return g.counter.Add(1)
}
