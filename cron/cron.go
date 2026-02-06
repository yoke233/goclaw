package cron

import (
	"time"
)

// Cron 定时任务管理器
type Cron struct {
	jobs map[string]*ScheduledJob
	stop chan struct{}
}

// ScheduledJob 定时任务
type ScheduledJob struct {
	ID       string
	Schedule Schedule
	Func     func()
	Next     time.Time
}

// NewCron 创建 Cron
func NewCron() *Cron {
	return &Cron{
		jobs: make(map[string]*ScheduledJob),
		stop: make(chan struct{}),
	}
}

// Schedule 调度接口
type Schedule interface {
	Next(time.Time) time.Time
}

// ScheduleFunc 调度函数
type ScheduleFunc func(time.Time) time.Time

// Next 实现 Schedule 接口
func (f ScheduleFunc) Next(t time.Time) time.Time {
	return f(t)
}

// Run 运行 Cron
func (c *Cron) Run() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stop:
			return
		case now := <-ticker.C:
			for _, job := range c.jobs {
				if now.After(job.Next) || now.Equal(job.Next) {
					go job.Func()
					job.Next = job.Schedule.Next(now)
				}
			}
		}
	}
}

// Stop 停止 Cron
func (c *Cron) Stop() {
	close(c.stop)
}

// Schedule 添加调度
func (c *Cron) Schedule(schedule Schedule, jobFunc func(), id string) {
	next := schedule.Next(time.Now())
	c.jobs[id] = &ScheduledJob{
		ID:       id,
		Schedule: schedule,
		Func:     jobFunc,
		Next:     next,
	}
}

// Remove 移除调度
func (c *Cron) Remove(id string) {
	delete(c.jobs, id)
}

// Parse 解析 cron 表达式（简化版）
func Parse(spec string) (Schedule, error) {
	// 简化实现：只支持 "every X minutes" 格式
	// 实际应该使用完整的 cron 解析器
	return ScheduleFunc(func(t time.Time) time.Time {
		return t.Add(time.Minute)
	}), nil
}
