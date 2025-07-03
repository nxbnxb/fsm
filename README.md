# 有限状态自动机（FSM）库

## 简介
本库实现了一个通用的有限状态自动机（Finite State Machine, FSM），支持泛型上下文数据，可用于管理和控制复杂系统中的状态转换。通过简洁的 API，你可以轻松定义状态、事件、转换规则以及回调函数，实现状态机的各种功能。

## 主要特性
1. **泛型支持**：使用 Go 泛型，允许存储任意类型的上下文数据。
2. **链式调用**：所有状态和转换添加方法都支持链式调用，提高代码的可读性和简洁性。
3. **错误处理**：将错误信息存储在上下文对象中，方便在链式调用结束后统一处理。
4. **状态转换规则**：支持有条件和无条件的状态转换，可自定义转换条件和回调函数。
5. **回调函数**：支持进入状态、退出状态、事件处理以及全局的前后回调函数。
6. **并发安全**：使用读写锁保证状态机在并发环境下的安全访问。

## 安装
```sh
go get github.com/nxbnxb/fsm
```

## 使用示例

### 1. 定义状态和事件
```go
const (
    StateInitial   fsm.State = "initial"
    StatePending   fsm.State = "pending"
    StateProcessed fsm.State = "processed"
    StateFailed    fsm.State = "failed"

    EventStart   fsm.Event = "start"
    EventProcess fsm.Event = "process"
    EventFail    fsm.Event = "fail"
)
```

### 2. 定义上下文数据类型
```go
type TestContext struct {
    Counter int
    Data    string
}
```

### 3. 创建状态机并配置
```go
machine := fsm.NewStateMachine[TestContext](StateInitial)
machine.AddSimpleTransition(StateInitial, StatePending, EventStart).
        OnEnterState(StatePending, func(ctx *fsm.Context[TestContext]) error {
                // 处理进入状态的逻辑
                return nil
        })
```

### 4. 处理错误
```go
if machine.ctx.Error != nil {
    // 处理错误
    fmt.Println("配置状态机出错:", machine.ctx.Error)
}
```

### 5. 发送事件
```go
if err := machine.SendEvent(EventStart); err != nil {
    fmt.Println("发送事件出错:", err)
}
```

## API 文档

### 状态机创建
```go
func NewStateMachine[T any](initialState State, opts ...Option[T]) *StateMachine[T]
```
- **参数**：
  - `initialState`：初始状态。
  - `opts`：可选配置选项。
- **返回值**：新创建的状态机实例。

### 配置选项
- `WithImplicitStateCreation(enabled bool)`：启用或禁用隐式状态创建。
- `WithLogger(logger *slog.Logger)`：设置日志记录器。
- `WithGlobalBeforeEvent(callback TransitionFunc[T])`：设置全局事件处理前的回调函数。
- `WithGlobalAfterEvent(callback TransitionFunc[T])`：设置全局事件处理后的回调函数。
- `WithGlobalBeforeTransition(callback TransitionFunc[T])`：设置全局状态转换前的回调函数。
- `WithGlobalAfterTransition(callback TransitionFunc[T])`：设置全局状态转换后的回调函数。

### 状态和转换管理
- `AddState(state State) *StateMachine[T]`：添加一个新状态。
- `AddTransition(from, to State, event Event, condition ConditionFunc[T]) *StateMachine[T]`：添加一个有条件的状态转换规则。
- `AddSimpleTransition(from, to State, event Event) *StateMachine[T]`：添加一个无条件的状态转换规则。
- `AddTransitionWithCallback(from, to State, event Event, condition ConditionFunc[T], callback TransitionFunc[T]) *StateMachine[T]`：添加一个带回调的有条件状态转换规则。
- `AddSimpleTransitionWithCallback(from, to State, event Event, callback TransitionFunc[T]) *StateMachine[T]`：添加一个带回调的无条件状态转换规则。

### 回调函数注册
- `OnEvent(state State, event Event, callback TransitionFunc[T]) *StateMachine[T]`：注册事件回调函数。
- `OnEnterState(state State, callback TransitionFunc[T]) *StateMachine[T]`：注册进入状态回调函数。
- `OnExitState(state State, callback TransitionFunc[T]) *StateMachine[T]`：注册退出状态回调函数。
- `OnStateChange(callback StateChangeCallback[T]) *StateMachine[T]`：注册状态变更回调函数。
- `BeforeAnyEvent(callback TransitionFunc[T]) *StateMachine[T]`：注册全局事件处理前的回调函数。
- `AfterAnyEvent(callback TransitionFunc[T]) *StateMachine[T]`：注册全局事件处理后的回调函数。

### 事件处理
```go
func (sm *StateMachine[T]) SendEvent(event Event) error
```
- **参数**：
  - `event`：要发送的事件。
- **返回值**：事件处理过程中可能出现的错误。

### 自动运行模式
```go
func (sm *StateMachine[T]) StartAutoRun(generator EventGenerator[T]) *StateMachine[T]
```
- **参数**：
  - `generator`：事件生成器函数。
- **返回值**：状态机实例，支持链式调用。

## 错误处理
所有添加状态、转换和回调函数的方法不再返回错误，而是将错误信息存储在 `Context` 对象的 `Error` 字段中。在链式调用结束后，你可以检查 `machine.ctx.Error` 来获取可能发生的错误信息。

## 并发安全
状态机使用读写锁（`sync.RWMutex`）来保护共享资源，确保在并发环境下的安全访问。你可以放心地在多个 goroutine 中调用状态机的方法。

## 测试
本库包含了一系列单元测试，覆盖了基本状态转换、带条件的转换、回调函数、全局回调、并发安全等功能。你可以使用以下命令运行测试：
```sh
go test ./...
```

## 贡献
如果你发现任何问题或有改进建议，欢迎提交 issue 或 pull request。请确保你的代码遵循 Go 语言的编码规范，并添加相应的测试用例。

## 许可证
本库采用 Apache License 2.0 许可证。详情请参阅 [LICENSE](LICENSE) 文件。