package central

import (
	"testing"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/fiber"
	"github.com/jerbe/et-go/module/login"
)

type stubAccountStoreComponent struct {
	ecs.BaseComponent
	store AccountStore
}

func (c *stubAccountStoreComponent) Type() string { return "CentralAccountStoreComponent" }
func (c *stubAccountStoreComponent) Store() AccountStore {
	return c.store
}

func TestCentralMailboxDispatch(t *testing.T) {
	passwordHash, err := HashPassword("p")
	if err != nil {
		t.Fatalf("HashPassword err = %v", err)
	}
	scene := ecs.NewScene(ecs.SceneTypeCentral, 1, "central")
	scene.AddComponent(&stubAccountStoreComponent{
		store: &stubAccountStore{
			account: &CAccount{
				Id:                2024,
				PasswordHash:      passwordHash,
				PasswordAlgorithm: PasswordAlgorithmArgon2id,
			},
		},
	})
	mailbox := actor.NewMailBox(actor.ActorID{ProcessID: 1, FiberID: 1, InstanceID: scene.InstanceID()}, actor.MailBoxTypeUnOrderedMessage)
	scene.AddComponent(mailbox)
	mailbox.RegisterHandler(MsgR2CentralAccountLogin, func(_ actor.ActorID, _ uint16, payload []byte) ([]byte, error) {
		req, err := unmarshalR2CentralAccountLogin(payload)
		if err != nil {
			return nil, err
		}
		resp, err := HandleAccountLogin(scene, req)
		if err != nil {
			return nil, err
		}
		return marshalCentral2RAccountLogin(resp)
	})

	payload, _ := marshalR2CentralAccountLogin(&R2CentralAccountLogin{RpcId: 7, Username: "u", Password: "p"})
	respPayload, err := actor.DispatchFiberMessage(scene, fiber.Message{
		To:      scene.InstanceID(),
		MsgID:   MsgR2CentralAccountLogin,
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("DispatchFiberMessage err = %v", err)
	}

	resp, err := unmarshalCentral2RAccountLogin(respPayload)
	if err != nil {
		t.Fatalf("Unmarshal err = %v", err)
	}
	accountID, err := login.VerifyAccessToken(resp.AccessToken)
	if err != nil {
		t.Fatalf("VerifyAccessToken err = %v", err)
	}
	if accountID != 2024 {
		t.Fatalf("accountID = %d, want 2024", accountID)
	}
}
