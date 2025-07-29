package ratelimit

import (
	"sync"
	"time"
)

// RPMLimiter RPM（每分钟请求数）限制器
type RPMLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*groupLimiter
}

// groupLimiter 分组限制器
type groupLimiter struct {
	limit     int           // 每分钟请求数限制
	requests  []time.Time   // 请求时间戳列表
	mu        sync.Mutex    // 保护requests切片
}

// NewRPMLimiter 创建新的RPM限制器
func NewRPMLimiter() *RPMLimiter {
	return &RPMLimiter{
		limiters: make(map[string]*groupLimiter),
	}
}

// SetLimit 设置分组的RPM限制
func (r *RPMLimiter) SetLimit(groupID string, limit int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	if limit <= 0 {
		// 如果限制为0或负数，删除限制器
		delete(r.limiters, groupID)
		return
	}
	
	r.limiters[groupID] = &groupLimiter{
		limit:    limit,
		requests: make([]time.Time, 0),
	}
}

// Allow 检查是否允许请求
func (r *RPMLimiter) Allow(groupID string) bool {
	r.mu.RLock()
	limiter, exists := r.limiters[groupID]
	r.mu.RUnlock()
	
	if !exists {
		// 没有设置限制，允许请求
		return true
	}
	
	return limiter.allow()
}

// allow 检查分组限制器是否允许请求
func (g *groupLimiter) allow() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	now := time.Now()
	oneMinuteAgo := now.Add(-time.Minute)
	
	// 清理一分钟前的请求记录
	validRequests := make([]time.Time, 0, len(g.requests))
	for _, reqTime := range g.requests {
		if reqTime.After(oneMinuteAgo) {
			validRequests = append(validRequests, reqTime)
		}
	}
	g.requests = validRequests
	
	// 检查是否超过限制
	if len(g.requests) >= g.limit {
		return false
	}
	
	// 记录当前请求
	g.requests = append(g.requests, now)
	return true
}

// GetStats 获取分组的统计信息
func (r *RPMLimiter) GetStats(groupID string) (current int, limit int, exists bool) {
	r.mu.RLock()
	limiter, exists := r.limiters[groupID]
	r.mu.RUnlock()
	
	if !exists {
		return 0, 0, false
	}
	
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	
	now := time.Now()
	oneMinuteAgo := now.Add(-time.Minute)
	
	// 计算当前一分钟内的请求数
	current = 0
	for _, reqTime := range limiter.requests {
		if reqTime.After(oneMinuteAgo) {
			current++
		}
	}
	
	return current, limiter.limit, true
}

// RemoveLimit 移除分组的RPM限制
func (r *RPMLimiter) RemoveLimit(groupID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.limiters, groupID)
}

// UpdateLimits 批量更新分组限制
func (r *RPMLimiter) UpdateLimits(limits map[string]int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// 清空现有限制器
	r.limiters = make(map[string]*groupLimiter)
	
	// 设置新的限制
	for groupID, limit := range limits {
		if limit > 0 {
			r.limiters[groupID] = &groupLimiter{
				limit:    limit,
				requests: make([]time.Time, 0),
			}
		}
	}
}

// GetAllStats 获取所有分组的统计信息
func (r *RPMLimiter) GetAllStats() map[string]map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	stats := make(map[string]map[string]int)
	now := time.Now()
	oneMinuteAgo := now.Add(-time.Minute)
	
	for groupID, limiter := range r.limiters {
		limiter.mu.Lock()
		
		// 计算当前一分钟内的请求数
		current := 0
		for _, reqTime := range limiter.requests {
			if reqTime.After(oneMinuteAgo) {
				current++
			}
		}
		
		stats[groupID] = map[string]int{
			"current": current,
			"limit":   limiter.limit,
		}
		
		limiter.mu.Unlock()
	}
	
	return stats
}
