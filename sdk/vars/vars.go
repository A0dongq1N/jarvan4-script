// Package vars 提供 spec.VarStore 接口的标准实现，由 Worker 注入到 RunContext。
// 每个 VU goroutine 持有独立实例，VU 间数据不共享。
package vars

import "sync"

// VarStore 实现 spec.VarStore 接口。
type VarStore struct {
	mu   sync.RWMutex
	data map[string]interface{}
	env  map[string]string // Master 下发的环境变量（只读）
}

// New 创建 VarStore，env 为 Master 下发的环境变量。
func New(env map[string]string) *VarStore {
	if env == nil {
		env = make(map[string]string)
	}
	return &VarStore{
		data: make(map[string]interface{}),
		env:  env,
	}
}

func (v *VarStore) Set(key string, value interface{}) {
	v.mu.Lock()
	v.data[key] = value
	v.mu.Unlock()
}

func (v *VarStore) Get(key string) interface{} {
	v.mu.RLock()
	val := v.data[key]
	v.mu.RUnlock()
	return val
}

func (v *VarStore) GetString(key string) string {
	val := v.Get(key)
	if val == nil {
		return ""
	}
	s, _ := val.(string)
	return s
}

func (v *VarStore) GetInt(key string) int {
	val := v.Get(key)
	if val == nil {
		return 0
	}
	i, _ := val.(int)
	return i
}

func (v *VarStore) Delete(key string) {
	v.mu.Lock()
	delete(v.data, key)
	v.mu.Unlock()
}

func (v *VarStore) Env(key string) string {
	return v.env[key]
}
