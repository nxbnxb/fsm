package example

import (
	"fmt"
	"testing"

	"github.com/nxbnxb/fsm" // 替换为实际包路径
)

// 订单状态机示例

// OrderState 定义订单状态类型
type OrderState string

// 订单状态常量
const (
	OrderCreated   OrderState = "created"
	OrderPaid      OrderState = "paid"
	OrderShipped   OrderState = "shipped"
	OrderCompleted OrderState = "completed"
	OrderCancelled OrderState = "cancelled"
)

// OrderEvent 定义订单事件类型
type OrderEvent string

// 订单事件常量
const (
	EventPay      OrderEvent = "pay"
	EventShip     OrderEvent = "ship"
	EventComplete OrderEvent = "complete"
	EventCancel   OrderEvent = "cancel"
	EventRefund   OrderEvent = "refund"
)

// OrderContext 订单上下文，传递状态转换时的额外数据
type OrderContext struct {
	OrderID string
	Amount  float64
}

func TestFSM(t *testing.T) {
	// 创建订单状态机实例
	orderFSM := fsm.NewFSM[OrderState, OrderEvent, OrderContext](OrderCreated)

	// 定义状态转换规则
	orderFSM.AddTransitions(
		fsm.Transition[OrderState, OrderEvent]{From: OrderCreated, Event: EventPay, To: OrderPaid},
		fsm.Transition[OrderState, OrderEvent]{From: OrderPaid, Event: EventShip, To: OrderShipped},
		fsm.Transition[OrderState, OrderEvent]{From: OrderShipped, Event: EventComplete, To: OrderCompleted},
		fsm.Transition[OrderState, OrderEvent]{From: OrderCreated, Event: EventCancel, To: OrderCancelled},
		fsm.Transition[OrderState, OrderEvent]{From: OrderPaid, Event: EventRefund, To: OrderCancelled},
	)

	// 注册状态进入钩子
	orderFSM.OnEnter(OrderPaid, func(ctx OrderContext, state OrderState) error {
		fmt.Printf("进入支付状态: %s\n", state)
		return nil
	})

	// 注册状态退出钩子
	orderFSM.OnExit(OrderCreated, func(ctx OrderContext, state OrderState) error {
		fmt.Printf("退出创建状态: %s\n", state)
		return nil
	})

	// 注册状态转换动作
	orderFSM.On(OrderCreated, EventPay, OrderPaid, func(ctx OrderContext, from OrderState, event OrderEvent, to OrderState) error {
		fmt.Printf("订单 %s 已支付 %.2f 元，状态从 %s 变为 %s\n", ctx.OrderID, ctx.Amount, from, to)
		// 这里可以添加实际的支付处理逻辑
		return nil
	})

	orderFSM.On(OrderPaid, EventShip, OrderShipped, func(ctx OrderContext, from OrderState, event OrderEvent, to OrderState) error {
		fmt.Printf("订单 %s 已发货，状态从 %s 变为 %s\n", ctx.OrderID, from, to)
		// 这里可以添加实际的发货处理逻辑
		return nil
	})

	// 使用状态机
	ctx := OrderContext{OrderID: "ORD-20230617", Amount: 199.99}

	fmt.Printf("订单初始状态: %s\n", orderFSM.CurrentState())

	// 执行完整状态转换流程
	executeFullWorkflow(orderFSM, ctx)

	// 打印状态历史
	fmt.Println("\n状态转换历史:")
	for i, entry := range orderFSM.History() {
		fmt.Printf("%d. %s -> %s [%s]\n", i+1, entry.From, entry.To, entry.Event)
	}
}

// executeFullWorkflow 执行完整的状态转换流程
func executeFullWorkflow(fsm *fsm.FSM[OrderState, OrderEvent, OrderContext], ctx OrderContext) {
	// 定义事件执行顺序
	events := []OrderEvent{EventPay, EventShip, EventComplete}

	for _, event := range events {
		current := fsm.CurrentState()

		fmt.Printf("\n当前状态: %s，尝试触发事件: %s\n", current, event)

		// 检查是否可以执行该事件
		if !fsm.Can(event) {
			fmt.Printf("错误: 状态 %s 下不能触发事件 %s\n", current, event)
			continue
		}

		// 执行转换
		if err := fsm.Transition(ctx, event); err != nil {
			fmt.Printf("转换失败: %v\n", err)
			break
		}

		fmt.Printf("转换成功，当前状态: %s\n", fsm.CurrentState())
	}
}
