package fsm

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

// State 表示自动机的状态类型
type State string

// Event 表示触发状态转换的事件类型
type Event string

// Context 是一个泛型类型，用于存储自动机的上下文数据
type Context[T any] struct {
	Data  T
	Error error
}

// TransitionFunc 是状态转换函数的签名
type TransitionFunc[T any] func(ctx *Context[T]) error

// TransitionEventFunc 是状态转换函数的签名
type TransitionEventFunc[T any] func(event Event, ctx *Context[T]) error

// ConditionFunc 是状态转换条件函数的签名
type ConditionFunc[T any] func(ctx *Context[T]) bool

// StateChangeCallback 是状态变更回调函数的签名
type StateChangeCallback[T any] func(oldState, newState State, event Event, ctx *Context[T])

// Option 是状态机配置选项的函数类型
type Option[T any] func(*StateMachine[T])

// globalCallbacks 封装全局回调函数
type globalCallbacks[T any] struct {
	beforeEvent      TransitionEventFunc[T]
	afterEvent       TransitionEventFunc[T]
	beforeTransition TransitionFunc[T]
	afterTransition  TransitionFunc[T]
}

// metrics 封装状态机指标
type metrics struct {
	eventTotal      *slog.Logger
	transitionTotal *slog.Logger
}

// StateMachine 是有限状态自动机的主要结构体
type StateMachine[T any] struct {
	mu                    sync.RWMutex                          // 保护共享资源
	currentState          State                                 // 当前状态
	states                map[State]struct{}                    // 所有注册的状态
	transitions           map[State]map[Event][]transition[T]   // 状态转换规则
	callbacks             map[State]map[Event]TransitionFunc[T] // 事件回调
	onEnterState          map[State]TransitionFunc[T]           // 进入状态回调
	onExitState           map[State]TransitionFunc[T]           // 退出状态回调
	ctx                   *Context[T]                           // 上下文数据
	implicitStateCreation bool                                  // 是否隐式创建状态
	globals               globalCallbacks[T]                    // 全局回调
	onStateChange         []StateChangeCallback[T]              // 状态变更回调
	logger                *slog.Logger                          // 日志记录器
	// metrics               *metrics                              // 指标收集器
	shutdownCallbacks []TransitionFunc[T] // 关闭回调
	running           bool                // 是否正在运行
}

// transition 表示一个状态转换规则
type transition[T any] struct {
	to        State
	condition ConditionFunc[T]
	callback  TransitionFunc[T]
}

// 自定义错误类型
var (
	ErrStateNotFound       = errors.New("状态不存在")
	ErrEventNoTransition   = errors.New("事件无匹配转换")
	ErrStateMachineRunning = errors.New("状态机正在运行")
)

// WithImplicitStateCreation 启用隐式状态创建
func WithImplicitStateCreation[T any](enabled bool) Option[T] {
	return func(sm *StateMachine[T]) {
		sm.implicitStateCreation = enabled
	}
}

// WithLogger 设置日志记录器
func WithLogger[T any](logger *slog.Logger) Option[T] {
	return func(sm *StateMachine[T]) {
		sm.logger = logger
	}
}

// WithGlobalBeforeEvent 设置在任何事件处理前执行的全局回调
func WithGlobalBeforeEvent[T any](callback TransitionEventFunc[T]) Option[T] {
	return func(sm *StateMachine[T]) {
		sm.globals.beforeEvent = callback
	}
}

// WithGlobalAfterEvent 设置在任何事件处理后执行的全局回调
func WithGlobalAfterEvent[T any](callback TransitionEventFunc[T]) Option[T] {
	return func(sm *StateMachine[T]) {
		sm.globals.afterEvent = callback
	}
}

// WithGlobalBeforeTransition 设置在任何状态转换前执行的全局回调
func WithGlobalBeforeTransition[T any](callback TransitionFunc[T]) Option[T] {
	return func(sm *StateMachine[T]) {
		sm.globals.beforeTransition = callback
	}
}

// WithGlobalAfterTransition 设置在任何状态转换后执行的全局回调
func WithGlobalAfterTransition[T any](callback TransitionFunc[T]) Option[T] {
	return func(sm *StateMachine[T]) {
		sm.globals.afterTransition = callback
	}
}

// NewStateMachine 创建一个新的有限状态自动机实例
func NewStateMachine[T any](initialState State, opts ...Option[T]) *StateMachine[T] {
	// 使用标准日志作为默认日志记录器
	logger := slog.Default()

	sm := &StateMachine[T]{
		currentState:          initialState,
		states:                make(map[State]struct{}),
		transitions:           make(map[State]map[Event][]transition[T]),
		callbacks:             make(map[State]map[Event]TransitionFunc[T]),
		onEnterState:          make(map[State]TransitionFunc[T]),
		onExitState:           make(map[State]TransitionFunc[T]),
		ctx:                   &Context[T]{},
		implicitStateCreation: true, // 默认启用隐式创建
		logger:                logger,
	}

	for _, opt := range opts {
		opt(sm)
	}

	// 隐式添加初始状态
	if sm.implicitStateCreation {
		sm.states[initialState] = struct{}{}
	}

	sm.logger.Info("状态机初始化完成", "initial_state", initialState)
	return sm
}

// AddState 添加一个新状态到自动机
func (sm *StateMachine[T]) AddState(state State) *StateMachine[T] {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.states[state] = struct{}{}
	sm.logger.Info("添加新状态", "state", state)
	return sm
}

// requireState 确保状态存在，如果不存在则根据配置决定是返回错误还是添加
func (sm *StateMachine[T]) requireState(state State, operation string) {
	_, exists := sm.states[state]
	if !exists {
		if sm.implicitStateCreation {
			sm.states[state] = struct{}{}
			sm.logger.Info("隐式创建状态", "state", state, "operation", operation)
		} else {
			sm.ctx.Error = fmt.Errorf("%w: %s: 状态 %s 不存在", ErrStateNotFound, operation, state)
		}
	}
}

// AddTransition 添加一个状态转换规则
func (sm *StateMachine[T]) AddTransition(from, to State, event Event, condition ConditionFunc[T]) *StateMachine[T] {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.requireState(from, "添加转换")
	if sm.ctx.Error != nil {
		return sm
	}
	sm.requireState(to, "添加转换")
	if sm.ctx.Error != nil {
		return sm
	}

	// 初始化事件转换映射
	if _, exists := sm.transitions[from]; !exists {
		sm.transitions[from] = make(map[Event][]transition[T])
	}

	sm.transitions[from][event] = append(sm.transitions[from][event], transition[T]{
		to:        to,
		condition: condition,
	})

	sm.logger.Info("添加状态转换", "from", from, "to", to, "event", event)
	return sm
}

// AddTransitionWithCallback 添加一个带回调的状态转换规则
func (sm *StateMachine[T]) AddTransitionWithCallback(from, to State, event Event, condition ConditionFunc[T], callback TransitionFunc[T]) *StateMachine[T] {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.requireState(from, "添加带回调转换")
	if sm.ctx.Error != nil {
		return sm
	}
	sm.requireState(to, "添加带回调转换")
	if sm.ctx.Error != nil {
		return sm
	}

	// 初始化事件转换映射
	if _, exists := sm.transitions[from]; !exists {
		sm.transitions[from] = make(map[Event][]transition[T])
	}

	sm.transitions[from][event] = append(sm.transitions[from][event], transition[T]{
		to:        to,
		condition: condition,
		callback:  callback,
	})

	sm.logger.Info("添加带回调状态转换", "from", from, "to", to, "event", event)
	return sm
}

// AddSimpleTransition 添加无条件的状态转换规则
func (sm *StateMachine[T]) AddSimpleTransition(from, to State, event Event) *StateMachine[T] {
	return sm.AddTransition(from, to, event, nil)
}

// AddSimpleTransitionWithCallback 添加无条件且带回调的状态转换规则
func (sm *StateMachine[T]) AddSimpleTransitionWithCallback(from, to State, event Event, callback TransitionFunc[T]) *StateMachine[T] {
	return sm.AddTransitionWithCallback(from, to, event, nil, callback)
}

// OnEvent 注册一个事件回调函数
func (sm *StateMachine[T]) OnEvent(state State, event Event, callback TransitionFunc[T]) *StateMachine[T] {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.requireState(state, "注册事件回调")
	if sm.ctx.Error != nil {
		return sm
	}

	// 初始化事件回调映射
	if _, exists := sm.callbacks[state]; !exists {
		sm.callbacks[state] = make(map[Event]TransitionFunc[T])
	}

	sm.callbacks[state][event] = callback
	sm.logger.Info("注册事件回调", "state", state, "event", event)
	return sm
}

// OnEnterState 注册一个状态进入回调函数
func (sm *StateMachine[T]) OnEnterState(state State, callback TransitionFunc[T]) *StateMachine[T] {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.requireState(state, "注册进入状态回调")
	if sm.ctx.Error != nil {
		return sm
	}

	sm.onEnterState[state] = callback
	sm.logger.Info("注册进入状态回调", "state", state)
	return sm
}

// OnExitState 注册一个状态退出回调函数
func (sm *StateMachine[T]) OnExitState(state State, callback TransitionFunc[T]) *StateMachine[T] {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.requireState(state, "注册退出状态回调")
	if sm.ctx.Error != nil {
		return sm
	}

	sm.onExitState[state] = callback
	sm.logger.Info("注册退出状态回调", "state", state)
	return sm
}

// OnStateChange 注册状态变更回调
func (sm *StateMachine[T]) OnStateChange(callback StateChangeCallback[T]) *StateMachine[T] {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.onStateChange = append(sm.onStateChange, callback)
	sm.logger.Info("注册状态变更回调")
	return sm
}

// BeforeAnyEvent 注册一个在所有事件处理前执行的全局回调
func (sm *StateMachine[T]) BeforeAnyEvent(callback TransitionEventFunc[T]) *StateMachine[T] {
	sm.globals.beforeEvent = callback
	return sm
}

// AfterAnyEvent 注册一个在所有事件处理后执行的全局回调
func (sm *StateMachine[T]) AfterAnyEvent(callback TransitionEventFunc[T]) *StateMachine[T] {
	sm.globals.afterEvent = callback
	return sm
}

// BeforeAnyTransition 注册一个在所有状态转换前执行的全局回调
func (sm *StateMachine[T]) BeforeAnyTransition(callback TransitionFunc[T]) *StateMachine[T] {
	sm.globals.beforeTransition = callback
	return sm
}

// AfterAnyTransition 注册一个在所有状态转换后执行的全局回调
func (sm *StateMachine[T]) AfterAnyTransition(callback TransitionFunc[T]) *StateMachine[T] {
	sm.globals.afterTransition = callback
	return sm
}

// OnShutdown 注册一个在状态机停止时执行的回调
func (sm *StateMachine[T]) OnShutdown(callback TransitionFunc[T]) *StateMachine[T] {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.shutdownCallbacks = append(sm.shutdownCallbacks, callback)
	sm.logger.Info("注册关闭回调")
	return sm
}

// SetContext 设置自动机的上下文数据
func (sm *StateMachine[T]) SetContext(data T) *StateMachine[T] {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.ctx.Data = data
	sm.logger.Info("设置上下文数据")
	return sm
}

// GetContext 获取自动机的上下文数据
func (sm *StateMachine[T]) GetContext() *Context[T] {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.ctx
}

// CurrentState 返回自动机的当前状态
func (sm *StateMachine[T]) CurrentState() State {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.currentState
}

// IsRunning 返回状态机是否正在运行
func (sm *StateMachine[T]) IsRunning() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.running
}

// SendEvent 向自动机发送一个事件，触发状态转换
func (sm *StateMachine[T]) SendEvent(event Event) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 确保 afterAnyEvent 始终执行
	defer func() {
		if sm.globals.afterEvent != nil {
			if err := sm.globals.afterEvent(event, sm.ctx); err != nil {
				sm.logger.Warn("全局事件后回调出错", "error", err)
			}
		}
	}()

	sm.logger.Info("收到事件", "event", event, "current_state", sm.currentState)

	// 执行全局事件前回调
	if sm.globals.beforeEvent != nil {
		if err := sm.globals.beforeEvent(event, sm.ctx); err != nil {
			return fmt.Errorf("全局事件前回调出错: %w", err)
		}
	}

	// 查找匹配的转换规则
	transition, err := sm.findMatchingTransition(event)
	if err != nil {
		return err
	}

	// 执行状态转换
	if err := sm.executeTransition(transition, event); err != nil {
		return err
	}

	return nil
}

// findMatchingTransition 查找匹配的转换规则
func (sm *StateMachine[T]) findMatchingTransition(event Event) (*transition[T], error) {
	currentTransitions, exists := sm.transitions[sm.currentState]
	if !exists {
		return nil, fmt.Errorf("%w: 状态 %s 无事件 %s 的转换", ErrEventNoTransition, sm.currentState, event)
	}

	// 查找匹配的转换规则
	transitions, ok := currentTransitions[event]
	if !ok {
		return nil, fmt.Errorf("%w: 状态 %s 无事件 %s 的转换", ErrEventNoTransition, sm.currentState, event)
	}

	// 查找第一个满足条件的转换
	var matchedTransition *transition[T]
	for _, t := range transitions {
		if t.condition == nil || t.condition(sm.ctx) {
			matchedTransition = &t
			break
		}
	}

	if matchedTransition == nil {
		return nil, fmt.Errorf("状态 %s 没有满足条件的事件 %s 的转换", sm.currentState, event)
	}

	return matchedTransition, nil
}

// executeTransition 执行状态转换
func (sm *StateMachine[T]) executeTransition(t *transition[T], event Event) error {
	oldState := sm.currentState
	sm.logger.Info("开始状态转换", "from", oldState, "to", t.to, "event", event)

	// 执行全局转换前回调
	if sm.globals.beforeTransition != nil {
		if err := sm.globals.beforeTransition(sm.ctx); err != nil {
			return fmt.Errorf("全局转换前回调出错: %w", err)
		}
	}

	// 执行事件回调（优先使用转换中定义的回调）
	var eventCallback TransitionFunc[T]
	if t.callback != nil {
		eventCallback = t.callback
	} else if ec, exists := sm.callbacks[oldState][event]; exists {
		eventCallback = ec
	}

	if eventCallback != nil {
		if err := eventCallback(sm.ctx); err != nil {
			return fmt.Errorf("处理事件 %s 时出错: %w", event, err)
		}
	}

	// 执行状态退出回调
	if exitCallback, exists := sm.onExitState[oldState]; exists {
		if err := exitCallback(sm.ctx); err != nil {
			return fmt.Errorf("退出状态 %s 时出错: %w", oldState, err)
		}
	}

	// 执行状态进入回调
	if enterCallback, exists := sm.onEnterState[t.to]; exists {
		if err := enterCallback(sm.ctx); err != nil {
			return fmt.Errorf("进入状态 %s 时出错: %w", t.to, err)
		}
	}

	// 转换到新状态
	sm.currentState = t.to

	// 执行全局转换后回调
	if sm.globals.afterTransition != nil {
		if err := sm.globals.afterTransition(sm.ctx); err != nil {
			return fmt.Errorf("全局转换后回调出错: %w", err)
		}
	}

	// 触发所有状态变更回调
	for _, cb := range sm.onStateChange {
		cb(oldState, t.to, event, sm.ctx)
	}

	sm.logger.Info("状态转换完成", "from", oldState, "to", sm.currentState, "event", event)
	return nil
}

// Validate 验证自动机的配置是否有效
func (sm *StateMachine[T]) Validate() error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// 检查初始状态是否存在
	if _, exists := sm.states[sm.currentState]; !exists {
		return fmt.Errorf("初始状态 %s 不存在", sm.currentState)
	}

	// 检查所有转换的目标状态是否存在
	for fromState, events := range sm.transitions {
		for event, transitions := range events {
			for _, t := range transitions {
				if _, exists := sm.states[t.to]; !exists {
					return fmt.Errorf("状态 %s 的事件 %s 转换到不存在的状态 %s", fromState, event, t.to)
				}
			}
		}
	}

	// 检查所有回调的状态是否存在
	for state := range sm.callbacks {
		if _, exists := sm.states[state]; !exists {
			return fmt.Errorf("回调注册到不存在的状态 %s", state)
		}
	}

	// 检查所有进入状态回调的状态是否存在
	for state := range sm.onEnterState {
		if _, exists := sm.states[state]; !exists {
			return fmt.Errorf("进入状态回调注册到不存在的状态 %s", state)
		}
	}

	// 检查所有退出状态回调的状态是否存在
	for state := range sm.onExitState {
		if _, exists := sm.states[state]; !exists {
			return fmt.Errorf("退出状态回调注册到不存在的状态 %s", state)
		}
	}

	return nil
}

// MustValidate 验证自动机配置，如果无效则panic
func (sm *StateMachine[T]) MustValidate() error {
	if err := sm.Validate(); err != nil {
		return fmt.Errorf("自动机配置无效: %v", err)
	}
	return nil
}

// EventGenerator 是事件生成器函数的签名
type EventGenerator[T any] func() (Event, bool)

// StartAutoRun 启动自动运行模式，持续发送事件直到满足退出条件
func (sm *StateMachine[T]) StartAutoRun(generator EventGenerator[T]) *StateMachine[T] {
	sm.mu.Lock()
	if sm.running {
		sm.ctx.Error = ErrStateMachineRunning
		sm.mu.Unlock()
		return sm
	}
	sm.running = true
	sm.mu.Unlock()

	for {
		event, ok := generator()
		if !ok {
			break
		}

		if err := sm.processEvent(event); err != nil {
			sm.ctx.Error = err
			break
		}
	}

	sm.mu.Lock()
	sm.running = false
	sm.mu.Unlock()
	return sm
}

// processEvent 处理单个事件
func (sm *StateMachine[T]) processEvent(event Event) error {
	sm.mu.RLock()
	currentState := sm.currentState
	transitions, ok := sm.transitions[currentState][event]
	sm.mu.RUnlock()

	if !ok {
		return ErrEventNoTransition
	}

	for _, trans := range transitions {
		if trans.condition == nil || trans.condition(sm.ctx) {
			sm.mu.Lock()
			oldState := sm.currentState
			sm.currentState = trans.to
			sm.mu.Unlock()

			if sm.globals.beforeTransition != nil {
				if err := sm.globals.beforeTransition(sm.ctx); err != nil {
					return err
				}
			}

			if exitCallback, ok := sm.onExitState[oldState]; ok {
				if err := exitCallback(sm.ctx); err != nil {
					return err
				}
			}

			if enterCallback, ok := sm.onEnterState[sm.currentState]; ok {
				if err := enterCallback(sm.ctx); err != nil {
					return err
				}
			}

			if trans.callback != nil {
				if err := trans.callback(sm.ctx); err != nil {
					return err
				}
			}

			if sm.globals.afterTransition != nil {
				if err := sm.globals.afterTransition(sm.ctx); err != nil {
					return err
				}
			}

			for _, callback := range sm.onStateChange {
				callback(oldState, sm.currentState, event, sm.ctx)
			}

			return nil
		}
	}

	return ErrEventNoTransition
}
