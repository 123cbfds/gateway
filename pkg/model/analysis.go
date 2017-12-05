package model

import (
	"context"
	"sync"
	"time"

	"github.com/fagongzi/log"
	"github.com/fagongzi/util/atomic"
	"github.com/fagongzi/util/task"
)

type point struct {
	requests          atomic.Int64
	rejects           atomic.Int64
	failure           atomic.Int64
	successed         atomic.Int64
	continuousFailure atomic.Int64

	costs atomic.Int64
	max   atomic.Int64
	min   atomic.Int64
}

func (p *point) dump(target *point) {
	target.requests.Set(p.requests.Get())
	target.rejects.Set(p.rejects.Get())
	target.failure.Set(p.failure.Get())
	target.successed.Set(p.successed.Get())
	target.max.Set(p.max.Get())
	target.min.Set(p.min.Get())
	target.costs.Set(p.costs.Get())

	p.min.Set(0)
	p.max.Set(0)
}

// Analysis analysis struct
type Analysis struct {
	sync.RWMutex

	taskRunner     *task.Runner
	points         map[string]*point
	recentlyPoints map[string]map[time.Duration]*Recently
}

// Recently recently point data
type Recently struct {
	period    time.Duration
	prev      *point
	current   *point
	dumpCurr  bool
	qps       int
	requests  int64
	successed int64
	failure   int64
	rejects   int64
	max       int64
	min       int64
	avg       int64
}

func newRecently(period time.Duration) *Recently {
	return &Recently{
		prev:    newPoint(),
		current: newPoint(),
		period:  period,
	}
}

func newPoint() *point {
	return &point{}
}

// NewAnalysis returns a Analysis
func NewAnalysis(taskRunner *task.Runner) *Analysis {
	return &Analysis{
		points:         make(map[string]*point),
		recentlyPoints: make(map[string]map[time.Duration]*Recently),
		taskRunner:     taskRunner,
	}
}

func (r *Recently) record(p *point) {
	if r.dumpCurr {
		p.dump(r.current)
		r.calc()
	} else {
		p.dump(r.prev)
	}

	r.dumpCurr = !r.dumpCurr
}

func (r *Recently) calc() {
	r.requests = r.current.requests.Get() - r.prev.requests.Get()

	if r.requests < 0 {
		r.requests = 0
	}

	r.successed = r.current.successed.Get() - r.prev.successed.Get()

	if r.successed < 0 {
		r.successed = 0
	}

	r.failure = r.current.failure.Get() - r.prev.failure.Get()

	if r.failure < 0 {
		r.failure = 0
	}

	r.rejects = r.current.rejects.Get() - r.prev.rejects.Get()

	if r.rejects < 0 {
		r.rejects = 0
	}

	r.max = r.current.max.Get()

	if r.max < 0 {
		r.max = 0
	} else {
		r.max = int64(r.max / 1000 / 1000)
	}

	r.min = r.current.min.Get()

	if r.min < 0 {
		r.min = 0
	} else {
		r.min = int64(r.min / 1000 / 1000)
	}

	costs := r.current.costs.Get() - r.prev.costs.Get()

	if r.requests == 0 {
		r.avg = 0
	} else {
		r.avg = int64(costs / 1000 / 1000 / r.requests)
	}

	if r.successed > r.requests {
		r.qps = int(r.requests / int64(r.period/time.Second))
	} else {
		r.qps = int(r.successed / int64(r.period/time.Second))
	}

}

// AddRecentCount add analysis point on a key
func (a *Analysis) AddRecentCount(key string, interval time.Duration) {
	a.Lock()
	defer a.Unlock()

	if interval == 0 {
		return
	}

	if _, ok := a.points[key]; !ok {
		a.points[key] = &point{}
	}

	if _, ok := a.recentlyPoints[key]; !ok {
		a.recentlyPoints[key] = make(map[time.Duration]*Recently)
	}

	if _, ok := a.recentlyPoints[key][interval]; ok {
		log.Infof("analysis: already added, key=<%s> interval=<%s>",
			key,
			interval)
		return
	}

	recently := newRecently(interval)
	a.recentlyPoints[key][interval] = recently
	timer := time.NewTicker(interval)

	a.taskRunner.RunCancelableTask(func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				timer.Stop()
				log.Infof("stop: analysis stopped, key=<%s> interval=<%s>",
					key,
					interval)
			case <-timer.C:
				p, ok := a.points[key]

				if ok {
					recently.record(p)
				}
			}
		}
	})

	log.Infof("analysis: added, key=<%s> interval=<%s>",
		key,
		interval)
}

// GetRecentlyRequestCount return the server request count in spec duration
func (a *Analysis) GetRecentlyRequestCount(server string, interval time.Duration) int {
	a.RLock()
	defer a.RUnlock()

	points, ok := a.recentlyPoints[server]
	if !ok {
		return 0
	}

	point, ok := points[interval]
	if !ok {
		return 0
	}

	return int(point.requests)
}

// GetRecentlyMax return max latency in spec secs
func (a *Analysis) GetRecentlyMax(server string, interval time.Duration) int {
	a.RLock()
	defer a.RUnlock()

	points, ok := a.recentlyPoints[server]
	if !ok {
		return 0
	}

	point, ok := points[interval]
	if !ok {
		return 0
	}

	return int(point.max)
}

// GetRecentlyMin return min latency in spec duration
func (a *Analysis) GetRecentlyMin(server string, interval time.Duration) int {
	a.RLock()
	defer a.RUnlock()

	points, ok := a.recentlyPoints[server]
	if !ok {
		return 0
	}

	point, ok := points[interval]
	if !ok {
		return 0
	}

	return int(point.min)
}

// GetRecentlyAvg return avg latency in spec secs
func (a *Analysis) GetRecentlyAvg(server string, interval time.Duration) int {
	a.RLock()
	defer a.RUnlock()

	points, ok := a.recentlyPoints[server]
	if !ok {
		return 0
	}

	point, ok := points[interval]
	if !ok {
		return 0
	}

	return int(point.avg)
}

// GetQPS return qps in spec duration
func (a *Analysis) GetQPS(server string, interval time.Duration) int {
	a.RLock()
	defer a.RUnlock()

	points, ok := a.recentlyPoints[server]
	if !ok {
		return 0
	}

	point, ok := points[interval]
	if !ok {
		return 0
	}

	return int(point.qps)
}

// GetRecentlyRejectCount return reject count in spec duration
func (a *Analysis) GetRecentlyRejectCount(server string, interval time.Duration) int {
	a.RLock()
	defer a.RUnlock()

	points, ok := a.recentlyPoints[server]
	if !ok {
		return 0
	}

	point, ok := points[interval]
	if !ok {
		return 0
	}

	return int(point.rejects)
}

// GetRecentlyRequestSuccessedCount return successed request count in spec secs
func (a *Analysis) GetRecentlyRequestSuccessedCount(server string, interval time.Duration) int {
	a.RLock()
	defer a.RUnlock()

	points, ok := a.recentlyPoints[server]
	if !ok {
		return 0
	}

	point, ok := points[interval]
	if !ok {
		return 0
	}

	return int(point.successed)
}

// GetRecentlyRequestFailureCount return failure request count in spec duration
func (a *Analysis) GetRecentlyRequestFailureCount(server string, interval time.Duration) int {
	a.RLock()
	defer a.RUnlock()

	points, ok := a.recentlyPoints[server]
	if !ok {
		return 0
	}

	point, ok := points[interval]
	if !ok {
		return 0
	}

	return int(point.failure)
}

// GetContinuousFailureCount return Continuous failure request count in spec secs
func (a *Analysis) GetContinuousFailureCount(server string) int {
	a.RLock()
	defer a.RUnlock()

	p, ok := a.points[server]
	if !ok {
		return 0
	}

	return int(p.continuousFailure.Get())
}

// Reject incr reject count
func (a *Analysis) Reject(key string) {
	a.Lock()
	p := a.points[key]
	p.rejects.Incr()
	a.Unlock()
}

// Failure incr failure count
func (a *Analysis) Failure(key string) {
	a.Lock()
	p := a.points[key]
	p.failure.Incr()
	p.continuousFailure.Incr()
	a.Unlock()
}

// Request incr request count
func (a *Analysis) Request(key string) {
	a.Lock()
	p := a.points[key]
	p.requests.Incr()
	a.Unlock()
}

// Response incr successed count
func (a *Analysis) Response(key string, cost int64) {
	a.Lock()
	p := a.points[key]
	p.successed.Incr()
	p.costs.Add(cost)
	p.continuousFailure.Set(0)

	if p.max.Get() < cost {
		p.max.Set(cost)
	}

	if p.min.Get() == 0 || p.min.Get() > cost {
		p.min.Set(cost)
	}
	a.Unlock()
}
