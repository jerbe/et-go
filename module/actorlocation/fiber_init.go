package actorlocation

import (
	"errors"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/coroutinelock"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
)

func init() {
	fiber.RegisterFiberInit(ecs.SceneTypeLocation, InitLocationFiber)
}

// InitLocationFiber 初始化位置服务所需组件。
func InitLocationFiber(f *fiber.Fiber) error {
	scene := f.Root()
	lockComponent := &coroutinelock.CoroutineLockComponent{}
	scene.AddComponent(lockComponent)

	manager := &LocationManagerComponent{}
	manager.SetLock(lockComponent.Lock())
	scene.AddComponent(manager)

	mailbox := actor.NewMailBox(actor.ActorID{
		ProcessID:  f.ProcessID(),
		FiberID:    f.ID(),
		InstanceID: scene.InstanceID(),
	}, actor.MailBoxTypeUnOrderedMessage)
	registerLocationHandlers(mailbox, manager)
	scene.AddComponent(mailbox)
	actor.UpdateSceneRegistry(scene)
	return nil
}

func registerLocationHandlers(mailbox *actor.MailBox, manager *LocationManagerComponent) {
	if mailbox == nil || manager == nil {
		return
	}

	mailbox.RegisterHandler(MsgObjectAddRequest, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalAddRequest(payload)
		if err != nil {
			return nil, err
		}
		resp := HandleAdd(manager, *req)
		return marshalAddResponse(&resp)
	})

	mailbox.RegisterHandler(MsgObjectGetRequest, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalGetRequest(payload)
		if err != nil {
			return nil, err
		}
		resp, err := HandleGet(manager, *req)
		if err != nil {
			resp.Error = 1
			if errors.Is(err, ErrLocationLocked) {
				resp.Error = ErrorCodeLocationLocked
			}
			resp.Message = err.Error()
		}
		return marshalGetResponse(&resp)
	})

	mailbox.RegisterHandler(MsgObjectLockRequest, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalLockRequest(payload)
		if err != nil {
			return nil, err
		}
		resp := HandleLock(manager, *req)
		return marshalLockResponse(&resp)
	})

	mailbox.RegisterHandler(MsgObjectUnlockRequest, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalUnlockRequest(payload)
		if err != nil {
			return nil, err
		}
		resp := HandleUnlock(manager, *req)
		return marshalUnlockResponse(&resp)
	})

	mailbox.RegisterHandler(MsgObjectRemoveRequest, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalRemoveRequest(payload)
		if err != nil {
			return nil, err
		}
		resp := HandleRemove(manager, *req)
		return marshalRemoveResponse(&resp)
	})
}
