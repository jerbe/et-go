package actor

import (
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
)

// DispatchFiberMessage 将 Fiber 消息路由到目标实体的 MailBox。
func DispatchFiberMessage(scene *ecs.Scene, message fiber.Message) ([]byte, error) {
	if scene == nil {
		return nil, ErrActorNotFound
	}

	entity, ok := scene.GetEntity(message.To)
	if !ok || entity == nil {
		return nil, ErrActorNotFound
	}

	component, ok := entity.GetComponent("MailBox")
	if !ok || component == nil {
		return nil, ErrActorNotFound
	}

	mailbox, ok := component.(*MailBox)
	if !ok || mailbox == nil {
		return nil, ErrActorNotFound
	}

	return mailbox.Dispatch(message.MsgID, message.Payload)
}
