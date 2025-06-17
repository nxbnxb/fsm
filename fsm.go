package fsm


// generate for doubao
import (
    "container/list"
    "fmt"
)

// Event 表示触发状态转换的事件类型
type Event string

// State 表示状态机的状态类型
type State string

// Transition 表示状态转换规则
type Transition[S comparable, E comparable] struct {
    From  S
    Event E
    To    S
}

// Action 是状态转换时执行的回调函数
type Action[S comparable, E comparable, C any] func(ctx C, from S, event E, to S) error

// StateHook 是状态进入或退出时执行的回调函数
type StateHook[S comparable, C any] func(ctx C, state S) error

// HistoryEntry 记录状态转换历史
type HistoryEntry[S comparable, E comparable] struct {
    From  S
    Event E
    To    S
}

// FSM 是泛型有限状态机的主结构体
type FSM[S comparable, E comparable, C any] struct {
    currentState S
    transitions  map[S]map[E]S
    actions      map[Transition[S, E]]Action[S, E, C]
    enterHooks   map[S][]StateHook[S, C]
    exitHooks    map[S][]StateHook[S, C]
    history      *list.List // 状态转换历史
    maxHistory   int        // 最大历史记录数
}

// NewFSM 创建一个新的有限状态机实例
func NewFSM[S comparable, E comparable, C any](initialState S) *FSM[S, E, C] {
    return &FSM[S, E, C]{
        currentState: initialState,
        transitions:  make(map[S]map[E]S),
        actions:      make(map[Transition[S, E]]Action[S, E, C]),
        enterHooks:   make(map[S][]StateHook[S, C]),
        exitHooks:    make(map[S][]StateHook[S, C]),
        history:      list.New(),
        maxHistory:   100, // 默认最大历史记录数
    }
}

// SetMaxHistory 设置最大历史记录数
func (f *FSM[S, E, C]) SetMaxHistory(max int) {
    f.maxHistory = max
}

// AddTransition 添加单个状态转换规则
func (f *FSM[S, E, C]) AddTransition(from S, event E, to S) {
    if _, ok := f.transitions[from]; !ok {
        f.transitions[from] = make(map[E]S)
    }
    f.transitions[from][event] = to
}

// AddTransitions 批量添加状态转换规则
func (f *FSM[S, E, C]) AddTransitions(transitions ...Transition[S, E]) {
    for _, t := range transitions {
        f.AddTransition(t.From, t.Event, t.To)
    }
}

// On 添加状态转换时执行的动作
func (f *FSM[S, E, C]) On(from S, event E, to S, action Action[S, E, C]) {
    transition := Transition[S, E]{From: from, Event: event, To: to}
    f.actions[transition] = action
}

// OnEnter 添加状态进入时执行的钩子
func (f *FSM[S, E, C]) OnEnter(state S, hook StateHook[S, C]) {
    f.enterHooks[state] = append(f.enterHooks[state], hook)
}

// OnExit 添加状态退出时执行的钩子
func (f *FSM[S, E, C]) OnExit(state S, hook StateHook[S, C]) {
    f.exitHooks[state] = append(f.exitHooks[state], hook)
}

// CurrentState 返回当前状态
func (f *FSM[S, E, C]) CurrentState() S {
    return f.currentState
}

// Can 检查是否可以从当前状态通过特定事件转换
func (f *FSM[S, E, C]) Can(event E) bool {
    if transitions, ok := f.transitions[f.currentState]; ok {
        _, ok := transitions[event]
        return ok
    }
    return false
}

// Events 返回当前状态下可用的事件列表
func (f *FSM[S, E, C]) Events() []E {
    if transitions, ok := f.transitions[f.currentState]; ok {
        events := make([]E, 0, len(transitions))
        for event := range transitions {
            events = append(events, event)
        }
        return events
    }
    return []E{}
}

// History 返回状态转换历史
func (f *FSM[S, E, C]) History() []HistoryEntry[S, E] {
    history := make([]HistoryEntry[S, E], 0, f.history.Len())
    for e := f.history.Front(); e != nil; e = e.Next() {
        history = append(history, e.Value.(HistoryEntry[S, E]))
    }
    return history
}

// Transition 执行状态转换
func (f *FSM[S, E, C]) Transition(ctx C, event E) error {
    if !f.Can(event) {
        return fmt.Errorf("transition from state %v on event %v is not allowed", f.currentState, event)
    }

    to := f.transitions[f.currentState][event]
    transition := Transition[S, E]{From: f.currentState, Event: event, To: to}

    // 执行退出当前状态的钩子
    if hooks, ok := f.exitHooks[f.currentState]; ok {
        for _, hook := range hooks {
            if err := hook(ctx, f.currentState); err != nil {
                return fmt.Errorf("exit hook failed for state %v: %w", f.currentState, err)
            }
        }
    }

    // 执行状态转换前的动作
    if action, ok := f.actions[transition]; ok {
        if err := action(ctx, f.currentState, event, to); err != nil {
            return fmt.Errorf("action failed during transition from %v to %v on event %v: %w",
                f.currentState, to, event, err)
        }
    }

    // 记录历史
    f.history.PushBack(HistoryEntry[S, E]{
        From:  f.currentState,
        Event: event,
        To:    to,
    })

    // 限制历史记录数量
    if f.history.Len() > f.maxHistory {
        f.history.Remove(f.history.Front())
    }

    // 执行进入新状态的钩子
    if hooks, ok := f.enterHooks[to]; ok {
        for _, hook := range hooks {
            if err := hook(ctx, to); err != nil {
                return fmt.Errorf("enter hook failed for state %v: %w", to, err)
            }
        }
    }

    // 更新当前状态
    f.currentState = to
    return nil
}    
