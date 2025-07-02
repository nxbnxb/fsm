package fsm_test

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	fsm "github.com/nxbnxb/fsm"
)

// 测试状态和事件常量
const (
	StateInitial   fsm.State = "initial"
	StatePending   fsm.State = "pending"
	StateProcessed fsm.State = "processed"
	StateFailed    fsm.State = "failed"

	EventStart   fsm.Event = "start"
	EventProcess fsm.Event = "process"
	EventFail    fsm.Event = "fail"
)

// 测试上下文数据类型
type TestContext struct {
	Counter int
	Data    string
}

// 测试基本状态转换
func TestBasicStateTransition(t *testing.T) {
	// 创建状态机
	machine := fsm.NewStateMachine[TestContext](StateInitial)

	// 添加状态转换规则
	machine.AddSimpleTransition(StateInitial, StatePending, EventStart)
	machine.AddSimpleTransition(StatePending, StateProcessed, EventProcess)
	machine.AddSimpleTransition(StatePending, StateFailed, EventFail)

	// 验证初始状态
	if machine.CurrentState() != StateInitial {
		t.Errorf("初始状态应为 %s，实际为 %s", StateInitial, machine.CurrentState())
	}

	// 发送事件并验证状态转换
	if err := machine.SendEvent(EventStart); err != nil {
		t.Fatalf("发送事件 %s 失败: %v", EventStart, err)
	}
	if machine.CurrentState() != StatePending {
		t.Errorf("状态应为 %s，实际为 %s", StatePending, machine.CurrentState())
	}

	if err := machine.SendEvent(EventProcess); err != nil {
		t.Fatalf("发送事件 %s 失败: %v", EventProcess, err)
	}
	if machine.CurrentState() != StateProcessed {
		t.Errorf("状态应为 %s，实际为 %s", StateProcessed, machine.CurrentState())
	}
}

// 测试带条件的状态转换
func TestConditionalTransition(t *testing.T) {
	machine := fsm.NewStateMachine[TestContext](StateInitial)

	// 添加带条件的转换
	machine.AddTransition(
		StateInitial,
		StatePending,
		EventStart,
		func(ctx *fsm.Context[TestContext]) bool {
			return ctx.Data.Counter > 0
		},
	)

	// 条件不满足时转换失败
	ctx := machine.GetContext()
	ctx.Data = TestContext{Counter: 0}

	err := machine.SendEvent(EventStart)
	if err == nil {
		t.Fatalf("条件不满足时应失败，但转换成功")
	}

	// 条件满足时转换成功
	ctx.Data.Counter = 1
	if err := machine.SendEvent(EventStart); err != nil {
		t.Fatalf("发送事件 %s 失败: %v", EventStart, err)
	}
	if machine.CurrentState() != StatePending {
		t.Errorf("状态应为 %s，实际为 %s", StatePending, machine.CurrentState())
	}
}

// 测试状态回调
func TestStateCallbacks(t *testing.T) {
	machine := fsm.NewStateMachine[TestContext](StateInitial)

	var enterInitialCalled bool
	var exitInitialCalled bool
	var enterPendingCalled, exitPendingCalled bool

	// 设置状态回调
	machine.OnEnterState(StateInitial, func(ctx *fsm.Context[TestContext]) error {
		enterInitialCalled = true
		return nil
	})

	machine.OnExitState(StateInitial, func(ctx *fsm.Context[TestContext]) error {
		exitInitialCalled = true
		return nil
	})

	machine.OnEnterState(StatePending, func(ctx *fsm.Context[TestContext]) error {
		enterPendingCalled = true
		return nil
	})

	machine.OnExitState(StatePending, func(ctx *fsm.Context[TestContext]) error {
		exitPendingCalled = true
		return nil
	})

	// 添加转换规则
	machine.AddSimpleTransition(StateInitial, StatePending, EventStart)
	machine.AddSimpleTransition(StatePending, StateProcessed, EventProcess)

	if enterInitialCalled {
		slog.Info("enterInitialCalled", slog.Bool("", enterInitialCalled))
	}

	// 触发状态转换
	if err := machine.SendEvent(EventStart); err != nil {
		t.Fatalf("发送事件 %s 失败: %v", EventStart, err)
	}

	if !exitInitialCalled || !enterPendingCalled {
		t.Errorf("状态转换未触发预期的回调")
	}

	if err := machine.SendEvent(EventProcess); err != nil {
		t.Fatalf("发送事件 %s 失败: %v", EventProcess, err)
	}

	if !exitPendingCalled {
		t.Errorf("状态退出回调未触发")
	}
}

// 测试全局回调
func TestGlobalCallbacks(t *testing.T) {
	var beforeEventCalled, afterEventCalled bool
	var beforeTransitionCalled, afterTransitionCalled bool

	fsm := fsm.NewStateMachine(StateInitial,
		fsm.WithGlobalBeforeEvent(func(ctx *fsm.Context[TestContext]) error {
			beforeEventCalled = true
			return nil
		}),
		fsm.WithGlobalAfterEvent(func(ctx *fsm.Context[TestContext]) error {
			afterEventCalled = true
			return nil
		}),
		fsm.WithGlobalBeforeTransition(func(ctx *fsm.Context[TestContext]) error {
			beforeTransitionCalled = true
			return nil
		}),
		fsm.WithGlobalAfterTransition(func(ctx *fsm.Context[TestContext]) error {
			afterTransitionCalled = true
			return nil
		}),
	)

	fsm.AddSimpleTransition(StateInitial, StatePending, EventStart)

	if err := fsm.SendEvent(EventStart); err != nil {
		t.Fatalf("发送事件 %s 失败: %v", EventStart, err)
	}

	if !beforeEventCalled || !afterEventCalled || !beforeTransitionCalled || !afterTransitionCalled {
		t.Errorf("全局回调未全部触发")
	}
}

// 测试并发安全
func TestConcurrentAccess(t *testing.T) {
	machine := fsm.NewStateMachine[TestContext](StateInitial)
	machine.AddSimpleTransition(StateInitial, StatePending, EventStart)
	machine.AddSimpleTransition(StatePending, StateProcessed, EventProcess)

	var wg sync.WaitGroup
	numGoroutines := 100

	// 并发发送事件
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// 第一个goroutine发送EventStart
			if id == 0 {
				if err := machine.SendEvent(EventStart); err != nil {
					t.Errorf("goroutine %d: 发送事件 %s 失败: %v", id, EventStart, err)
				}
			} else {
				// 等待状态变为Pending
				for machine.CurrentState() != StatePending {
					time.Sleep(10 * time.Millisecond)
				}

				// 发送EventProcess
				if err := machine.SendEvent(EventProcess); err != nil {
					if !errors.Is(err, fsm.ErrEventNoTransition) {
						t.Errorf("goroutine %d: 发送事件 %s 失败: %v", id, EventProcess, err)
					}
				}
			}
		}(i)
	}

	wg.Wait()

	// 最终状态应为Processed或Pending（取决于竞争结果）
	if state := machine.CurrentState(); state != StateProcessed && state != StatePending {
		t.Errorf("最终状态应为 Processed 或 Pending，实际为 %s", state)
	}
}

// 测试自动运行模式
func TestAutoRun(t *testing.T) {
	machine := fsm.NewStateMachine[TestContext](StateInitial)
	machine.AddSimpleTransition(StateInitial, StatePending, EventStart)
	machine.AddSimpleTransition(StatePending, StateProcessed, EventProcess)

	// 事件生成器
	eventCount := 0
	eventGenerator := func() (fsm.Event, bool) {
		eventCount++
		switch eventCount {
		case 1:
			return EventStart, true
		case 2:
			return EventProcess, true
		default:
			return "", false // 停止自动运行
		}
	}

	// 启动自动运行
	if err := machine.StartAutoRun(eventGenerator); err != nil {
		t.Fatalf("自动运行失败: %v", err)
	}

	if machine.CurrentState() != StateProcessed {
		t.Errorf("最终状态应为 %s，实际为 %s", StateProcessed, machine.CurrentState())
	}
}

// 测试状态变更回调
func TestStateChangeCallback(t *testing.T) {
	machine := fsm.NewStateMachine[TestContext](StateInitial)
	machine.AddSimpleTransition(StateInitial, StatePending, EventStart)

	var oldState, newState fsm.State
	var event fsm.Event

	// 注册状态变更回调
	machine.OnStateChange(func(os, ns fsm.State, e fsm.Event, ctx *fsm.Context[TestContext]) {
		oldState = os
		newState = ns
		event = e
	})

	// 触发状态变更
	if err := machine.SendEvent(EventStart); err != nil {
		t.Fatalf("发送事件 %s 失败: %v", EventStart, err)
	}

	if oldState != StateInitial || newState != StatePending || event != EventStart {
		t.Errorf("状态变更回调参数不正确: 期望 (%s, %s, %s)，实际 (%s, %s, %s)",
			StateInitial, StatePending, EventStart, oldState, newState, event)
	}
}

// 测试验证功能
func TestValidation(t *testing.T) {
	// 有效配置
	fsmValid := fsm.NewStateMachine[TestContext](StateInitial)
	fsmValid.AddSimpleTransition(StateInitial, StatePending, EventStart)

	if err := fsmValid.Validate(); err != nil {
		t.Fatalf("有效配置验证失败: %v", err)
	}

	// 无效配置（初始状态不存在）
	fsmInvalid := fsm.NewStateMachine(StateInitial, fsm.WithImplicitStateCreation[TestContext](false))
	if err := fsmInvalid.Validate(); !errors.Is(err, fsm.ErrStateNotFound) {
		t.Fatalf("期望初始状态不存在错误，实际得到: %v", err)
	}

	// 无效配置（转换目标状态不存在）
	fsmInvalid2 := fsm.NewStateMachine[TestContext](StateInitial)
	fsmInvalid2.AddTransition(StateInitial, StatePending, EventStart, nil)
	// fsmInvalid2.states = make(map[fsm.State]struct{}) // 清空状态集

	if err := fsmInvalid2.Validate(); err == nil {
		t.Fatalf("期望验证失败，但验证通过")
	}
}

// 测试错误处理
func TestErrorHandling(t *testing.T) {
	machine := fsm.NewStateMachine[TestContext](StateInitial)

	// 尝试向不存在的转换发送事件
	err := machine.SendEvent(EventStart)
	if !errors.Is(err, fsm.ErrEventNoTransition) {
		t.Fatalf("期望事件无匹配转换错误，实际得到: %v", err)
	}

	// 添加转换但不添加目标状态（禁用隐式创建）
	fsmNoImplicit := fsm.NewStateMachine(StateInitial, fsm.WithImplicitStateCreation[TestContext](false))
	fsmNoImplicit = fsmNoImplicit.AddSimpleTransition(StateInitial, StatePending, EventStart)
	if !errors.Is(err, fsm.ErrStateNotFound) {
		t.Fatalf("期望状态不存在错误，实际得到: %v", err)
	}
	fmt.Println(fsmNoImplicit)
}

// 测试上下文数据
func TestContextData(t *testing.T) {
	machine := fsm.NewStateMachine[TestContext](StateInitial)

	// 设置上下文数据
	ctx := machine.GetContext()
	ctx.Data = TestContext{
		Counter: 100,
		Data:    "test data",
	}

	// 注册带上下文操作的转换回调
	machine.AddTransitionWithCallback(
		StateInitial,
		StatePending,
		EventStart,
		nil,
		func(ctx *fsm.Context[TestContext]) error {
			ctx.Data.Counter++
			ctx.Data.Data += " processed"
			return nil
		},
	)

	// 触发转换
	if err := machine.SendEvent(EventStart); err != nil {
		t.Fatalf("发送事件 %s 失败: %v", EventStart, err)
	}

	// 验证上下文数据
	if ctx.Data.Counter != 101 || ctx.Data.Data != "test data processed" {
		t.Errorf("上下文数据未正确更新: %+v", ctx.Data)
	}
}

// 测试自定义日志
func TestCustomLogger(t *testing.T) {
	var logOutput string
	logger := slog.New(slog.NewTextHandler(
		&testWriter{output: &logOutput},
		&slog.HandlerOptions{Level: slog.LevelDebug},
	))

	fsm := fsm.NewStateMachine(StateInitial, fsm.WithLogger[TestContext](logger))
	fsm.AddSimpleTransition(StateInitial, StatePending, EventStart)

	if err := fsm.SendEvent(EventStart); err != nil {
		t.Fatalf("发送事件失败: %v", err)
	}

	// 验证日志输出
	if logOutput == "" {
		t.Errorf("未生成日志输出")
	}
}

// 测试辅助类型 - 用于捕获日志输出
type testWriter struct {
	output *string
}

func (w *testWriter) Write(p []byte) (n int, err error) {
	*w.output += string(p)
	return len(p), nil
}

// 测试多事件路径
func TestMultipleEventPaths(t *testing.T) {
	machine := fsm.NewStateMachine[TestContext](StateInitial)

	// 添加两条从Initial到Pending的路径，条件不同
	machine.AddTransition(
		StateInitial,
		StatePending,
		EventStart,
		func(ctx *fsm.Context[TestContext]) bool {
			return ctx.Data.Counter < 10
		},
	).AddTransition(
		StateInitial,
		StateFailed,
		EventStart,
		func(ctx *fsm.Context[TestContext]) bool {
			return ctx.Data.Counter >= 10
		},
	)

	// 测试第一条路径
	ctx := machine.GetContext()
	ctx.Data.Counter = 5

	if err := machine.SendEvent(EventStart); err != nil {
		t.Fatalf("发送事件 %s 失败: %v", EventStart, err)
	}

	if machine.CurrentState() != StatePending {
		t.Errorf("状态应为 %s，实际为 %s", StatePending, machine.CurrentState())
	}

	// 重置状态并测试第二条路径
	machine = fsm.NewStateMachine[TestContext](StateInitial)
	machine.AddTransition(
		StateInitial,
		StatePending,
		EventStart,
		func(ctx *fsm.Context[TestContext]) bool {
			return ctx.Data.Counter < 10
		},
	)

	machine.AddTransition(
		StateInitial,
		StateFailed,
		EventStart,
		func(ctx *fsm.Context[TestContext]) bool {
			return ctx.Data.Counter >= 10
		},
	)

	ctx = machine.GetContext()
	ctx.Data.Counter = 15

	if err := machine.SendEvent(EventStart); err != nil {
		t.Fatalf("发送事件 %s 失败: %v", EventStart, err)
	}

	if machine.CurrentState() != StateFailed {
		t.Errorf("状态应为 %s，实际为 %s", StateFailed, machine.CurrentState())
	}
}
