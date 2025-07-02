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

// ConditionFunc 是状态转换条件函数的签名
type ConditionFunc[T any] func(ctx *Context[T]) bool

// StateChangeCallback 是状态变更回调函数的签名
type StateChangeCallback[T any] func(oldState, newState State, event Event, ctx *Context[T])

// Option 是状态机配置选项的函数类型
type Option[T any] func(*StateMachine[T])

// globalCallbacks 封装全局回调函数
type globalCallbacks[T any] struct {
	beforeEvent      TransitionFunc[T]
	afterEvent       TransitionFunc[T]
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
func WithGlobalBeforeEvent[T any](callback TransitionFunc[T]) Option[T] {
	return func(sm *StateMachine[T]) {
		sm.globals.beforeEvent = callback
	}
}

// WithGlobalAfterEvent 设置在任何事件处理后执行的全局回调
func WithGlobalAfterEvent[T any](callback TransitionFunc[T]) Option[T] {
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
func (sm *StateMachine[T]) BeforeAnyEvent(callback TransitionFunc[T]) *StateMachine[T] {
	sm.globals.beforeEvent = callback
	return sm
}

// AfterAnyEvent 注册一个在所有事件处理后执行的全局回调
func (sm *StateMachine[T]) AfterAnyEvent(callback TransitionFunc[T]) *StateMachine[T] {
	sm.globals.afterEvent = callback
	return sm
}
