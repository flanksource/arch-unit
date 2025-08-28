package shutdown

import (
	"container/heap"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/flanksource/commons/logger"
)

const (
	PriorityIngress  = 0
	PriorityDefault  = 100
	PriorityWorkers  = 200
	PriorityDatabase = 300
	PriorityCritical = 400
)

type Hook struct {
	label    string
	priority int
	fn       func()
	index    int // for heap interface
}

type HookHeap []*Hook

func (h HookHeap) Len() int           { return len(h) }
func (h HookHeap) Less(i, j int) bool { return h[i].priority < h[j].priority }
func (h HookHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *HookHeap) Push(x interface{}) {
	n := len(*h)
	item := x.(*Hook)
	item.index = n
	*h = append(*h, item)
}

func (h *HookHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	*h = old[0 : n-1]
	return item
}

var (
	hooks    HookHeap
	hooksMux sync.Mutex
	once     sync.Once
)

// AddHook registers a shutdown hook with default priority
func AddHook(label string, fn func()) {
	AddHookWithPriority(label, PriorityDefault, fn)
}

// AddHookWithPriority registers a shutdown hook with specific priority
func AddHookWithPriority(label string, priority int, fn func()) {
	hooksMux.Lock()
	defer hooksMux.Unlock()

	hook := &Hook{
		label:    label,
		priority: priority,
		fn:       fn,
	}
	heap.Push(&hooks, hook)
}

// Shutdown executes all registered hooks in priority order
func Shutdown() {
	hooksMux.Lock()
	defer hooksMux.Unlock()

	if len(hooks) == 0 {
		return
	}

	logger.Infof("Executing %d shutdown hooks", len(hooks))

	// Execute hooks in priority order (lowest priority first)
	for hooks.Len() > 0 {
		hook := heap.Pop(&hooks).(*Hook)
		logger.Debugf("Executing shutdown hook: %s (priority=%d)", hook.label, hook.priority)

		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("Panic in shutdown hook %s: %v", hook.label, r)
				}
			}()
			hook.fn()
		}()
	}

	logger.Infof("All shutdown hooks executed")
}

// WaitForSignal waits for interrupt signals and triggers shutdown
func WaitForSignal() {
	once.Do(func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		sig := <-sigChan
		fmt.Fprintf(os.Stderr, "\n🛑 Received %s - initiating graceful shutdown...\n", sig)
		fmt.Fprintf(os.Stderr, "   Press Ctrl+C again to force immediate exit\n\n")

		// Set up force exit on second signal
		go func() {
			<-sigChan
			fmt.Fprintf(os.Stderr, "\n⚠️  Force exit\n")
			os.Exit(1)
		}()

		Shutdown()
		os.Exit(0)
	})
}

// RunAndWait runs the provided function and then waits for shutdown signal
func RunAndWait(fn func() error) error {
	if err := fn(); err != nil {
		return err
	}
	WaitForSignal()
	return nil
}
