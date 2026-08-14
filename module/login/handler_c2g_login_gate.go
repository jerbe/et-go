package login

import (
	"context"
	"errors"
	"fmt"

	"github.com/jerbe/et-go/engine/actor"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/engine/network"
	"github.com/jerbe/et-go/engine/network/codec"
	"github.com/jerbe/et-go/module/actorlocation"
	"github.com/jerbe/et-go/module/gamelogin"
	"github.com/jerbe/et-go/module/gate"
	"github.com/jerbe/et-go/module/maprpc"
)

func init() {
	gate.RegisterSessionPacketHandler(MsgC2GLoginGate, func(scene *ecs.Scene, session *network.Session, packet *codec.Packet) (*codec.Packet, error) {
		req, err := unmarshalC2GLoginGate(packet.Payload)
		if err != nil {
			return nil, err
		}
		resp, err := HandleC2GLoginGate(scene, session, req)
		if err != nil {
			return nil, err
		}
		payload, err := marshalG2CLoginGate(resp)
		if err != nil {
			return nil, err
		}
		return &codec.Packet{
			Type:    codec.PacketTypeResponse,
			MsgID:   MsgG2CLoginGate,
			RpcID:   packet.RpcID,
			Payload: payload,
		}, nil
	})
}

// HandleC2GLoginGate 处理 Gate 登录请求。
func HandleC2GLoginGate(scene *ecs.Scene, session *network.Session, req *C2GLoginGate) (response *G2CLoginGate, err error) {
	if req == nil {
		return nil, ErrInvalidLoginRequest
	}
	if scene == nil || session == nil || session.Entity() == nil {
		return nil, gate.ErrSessionNil
	}

	component, ok := scene.GetComponent("GateSessionKeyComponent")
	if !ok || component == nil {
		return invalidGateLogin(session, req.RpcId), nil
	}
	keys, ok := component.(*GateSessionKeyComponent)
	if !ok {
		return invalidGateLogin(session, req.RpcId), nil
	}
	accountId, ok := keys.Get(req.Token)
	if !ok {
		return invalidGateLogin(session, req.RpcId), nil
	}
	keys.Remove(req.Token)
	accountLockHeld := true
	defer func() {
		if !accountLockHeld {
			return
		}
		if unlockErr := unlockAccountLocation(scene, accountId); unlockErr != nil && err == nil {
			response = nil
			err = unlockErr
		}
	}()
	if req.GateId <= 0 {
		return nil, ErrInvalidLoginRequest
	}
	if currentID := scene.ID(); currentID > 0 && req.GateId != currentID {
		return nil, ErrGateIDMismatch
	}

	callContext := session.Context()
	playerID, err := resolvePlayerIDWithContext(scene, accountId, callContext)
	if err != nil {
		return nil, err
	}
	if err := ensurePlayerUnitLocationWithContext(scene, playerID, callContext); err != nil {
		return &G2CLoginGate{
			RpcId:   req.RpcId,
			Error:   1,
			Message: err.Error(),
		}, nil
	}

	playerComponent, ok := scene.GetComponent("PlayerComponent")
	if !ok || playerComponent == nil {
		return nil, ErrInvalidLoginRequest
	}
	players, ok := playerComponent.(*PlayerComponent)
	if !ok || players == nil {
		return nil, ErrInvalidLoginRequest
	}

	player, exists := players.Get(accountId)
	if !exists || player == nil {
		player = NewPlayer(accountId, playerID)
		scene.AddChildWithID(playerID, &player.Entity)
		if err := players.Add(accountId, player); err != nil {
			player.Dispose()
			return nil, err
		}
	} else {
		player.UnitId = playerID
		player.SetID(playerID)
	}

	if err := ensureSessionInScene(scene, session); err != nil {
		return nil, err
	}
	if err := ensurePlayerMailbox(scene, player); err != nil {
		return nil, err
	}
	if err := bindPlayerSession(scene, player, session); err != nil {
		return nil, err
	}
	removeSessionAcceptTimeout(session)
	if err := ensureGateSessionMailbox(scene, session); err != nil {
		return nil, err
	}
	if err := registerLocations(scene, player, session); err != nil {
		return nil, err
	}
	if err := unlockAccountLocation(scene, accountId); err != nil {
		return nil, err
	}
	accountLockHeld = false
	session.MarkAuthed()

	return &G2CLoginGate{
		RpcId:          req.RpcId,
		PlayerId:       player.ID(),
		CharacterCount: 1,
	}, nil
}

func invalidGateLogin(session *network.Session, rpcID uint32) *G2CLoginGate {
	if session != nil {
		session.Close()
	}
	return &G2CLoginGate{
		RpcId:   rpcID,
		Error:   ERR_ConnectGateKeyError,
		Message: "Gate key验证失败!",
	}
}

func ensureSessionInScene(scene *ecs.Scene, session *network.Session) error {
	if scene == nil || session == nil || session.Entity() == nil {
		return gate.ErrSessionNil
	}
	if currentScene := session.Entity().Scene(); currentScene != nil {
		if currentScene != scene {
			return ErrInvalidLoginRequest
		}
		return nil
	}
	scene.AddChildWithID(session.ID(), session.Entity())
	return nil
}

func ensureGateSessionMailbox(scene *ecs.Scene, session *network.Session) error {
	if scene == nil || session == nil || session.Entity() == nil {
		return gate.ErrSessionNil
	}
	if component, ok := session.Entity().GetComponent("MailBox"); ok {
		if mailbox, ok := component.(*actor.MailBox); ok {
			mailbox.SetMailBoxType(actor.MailBoxTypeGateSession)
			mailbox.SetActorID(actorIDForEntity(scene, session.Entity()))
			return nil
		}
		return ErrInvalidLoginRequest
	}
	session.Entity().AddComponent(actor.NewMailBox(actorIDForEntity(scene, session.Entity()), actor.MailBoxTypeGateSession))
	return nil
}

func ensurePlayerMailbox(scene *ecs.Scene, player *Player) error {
	if scene == nil || player == nil {
		return ErrInvalidLoginRequest
	}
	if component, ok := player.GetComponent("MailBox"); ok {
		if _, valid := component.(*actor.MailBox); !valid {
			return ErrInvalidLoginRequest
		}
		return nil
	}
	player.AddComponent(actor.NewMailBox(actorIDForEntity(scene, &player.Entity), actor.MailBoxTypeUnOrderedMessage))
	return nil
}

func bindPlayerSession(scene *ecs.Scene, player *Player, session *network.Session) error {
	if scene == nil || player == nil || session == nil || session.Entity() == nil {
		return ErrInvalidLoginRequest
	}
	if component, ok := session.Entity().GetComponent("SessionPlayerComponent"); ok {
		if sessionPlayer, ok := component.(*SessionPlayerComponent); ok {
			sessionPlayer.Player = player
		} else {
			return ErrInvalidLoginRequest
		}
	} else {
		session.Entity().AddComponent(&SessionPlayerComponent{Player: player})
	}
	if component, ok := session.Entity().GetComponent("GateSessionComponent"); ok {
		if gateSession, valid := component.(*gate.GateSessionComponent); !valid || gateSession == nil {
			return ErrInvalidLoginRequest
		}
	} else {
		session.Entity().AddComponent(&gate.GateSessionComponent{Session: session})
	}
	if component, ok := player.GetComponent("PlayerSessionComponent"); ok {
		if playerSession, ok := component.(*PlayerSessionComponent); ok {
			playerSession.Session = session
		} else {
			return ErrInvalidLoginRequest
		}
	} else {
		player.AddComponent(&PlayerSessionComponent{Session: session})
	}
	return nil
}

func registerLocations(scene *ecs.Scene, player *Player, session *network.Session) error {
	if scene == nil || player == nil || session == nil || session.Entity() == nil {
		return ErrLocationRegistration
	}
	component, ok := scene.GetComponent("LocationProxyComponent")
	if !ok || component == nil {
		return ErrLocationProxyMissing
	}
	proxy, ok := component.(interface {
		Add(locationType int, key int64, actorID actor.ActorID) error
	})
	if !ok {
		return ErrLocationProxyMissing
	}

	playerActorID := actorIDForEntity(scene, &player.Entity)
	sessionActorID := actorIDForEntity(scene, session.Entity())
	if player.ID() > 0 {
		if err := proxy.Add(int(actorlocation.LocationTypePlayer), player.ID(), playerActorID); err != nil {
			return fmt.Errorf("%w: player: %v", ErrLocationRegistration, err)
		}
		if err := proxy.Add(int(actorlocation.LocationTypeGateSession), player.ID(), sessionActorID); err != nil {
			return fmt.Errorf("%w: gate session: %v", ErrLocationRegistration, err)
		}
		return nil
	}
	return ErrLocationRegistration
}

func removeSessionAcceptTimeout(session *network.Session) {
	if session == nil || session.Entity() == nil {
		return
	}
	session.Entity().RemoveComponent("SessionAcceptTimeoutComponent")
}

func unlockAccountLocation(scene *ecs.Scene, accountId int64) error {
	component, ok := scene.GetComponent("LocationProxyComponent")
	if !ok || component == nil {
		return ErrLocationProxyMissing
	}
	proxy, ok := component.(interface {
		Unlock(locationType int, key int64, oldActorID, newActorID actor.ActorID) error
	})
	if !ok {
		return ErrLocationProxyMissing
	}
	return proxy.Unlock(int(actorlocation.LocationTypeAccount), accountId, actor.ActorID{}, actor.ActorID{})
}

func resolvePlayerID(scene *ecs.Scene, accountId int64) (int64, error) {
	return resolvePlayerIDWithContext(scene, accountId, context.Background())
}

func resolvePlayerIDWithContext(scene *ecs.Scene, accountId int64, callContext context.Context) (int64, error) {
	if scene == nil || accountId <= 0 {
		return 0, ErrInvalidLoginRequest
	}
	return resolvePlayerIDFromCentralWithContext(scene, accountId, callContext)
}

func resolvePlayerIDFromCentral(scene *ecs.Scene, accountId int64) (int64, error) {
	return resolvePlayerIDFromCentralWithContext(scene, accountId, context.Background())
}

func resolvePlayerIDFromCentralWithContext(scene *ecs.Scene, accountId int64, callContext context.Context) (int64, error) {
	if scene == nil {
		return 0, ErrInvalidLoginRequest
	}
	component, ok := scene.GetComponent("MessageSender")
	if !ok || component == nil {
		return 0, ErrMessageSenderMissing
	}
	sender, ok := component.(interface {
		Call(ctx context.Context, actorID actor.ActorID, msgID uint16, payload []byte) ([]byte, error)
	})
	if !ok {
		return 0, ErrMessageSenderMissing
	}
	gameActorID, ok := actor.ResolveSceneActor(scene.Zone(), ecs.SceneTypeCentral, "")
	if !ok {
		return 0, ErrCentralActorMissing
	}
	payload, err := gamelogin.MarshalG2GameLogin(&gamelogin.G2GameLogin{
		RpcId:     1,
		AccountId: accountId,
	})
	if err != nil {
		return 0, err
	}
	if callContext == nil {
		callContext = context.Background()
	}
	respPayload, err := sender.Call(callContext, gameActorID, gamelogin.MsgG2GameLogin, payload)
	if err != nil {
		return 0, err
	}
	resp, err := gamelogin.UnmarshalGame2GLogin(respPayload)
	if err != nil {
		return 0, err
	}
	if resp.Error != 0 {
		if resp.Message == "" {
			resp.Message = "game login failed"
		}
		return 0, errors.New(resp.Message)
	}
	if resp.PlayerId <= 0 {
		return 0, ErrInvalidLoginRequest
	}
	return resp.PlayerId, nil
}

func ensurePlayerUnitLocation(scene *ecs.Scene, playerID int64) error {
	return ensurePlayerUnitLocationWithContext(scene, playerID, context.Background())
}

func ensurePlayerUnitLocationWithContext(scene *ecs.Scene, playerID int64, callContext context.Context) error {
	if scene == nil || playerID == 0 {
		return ErrInvalidLoginRequest
	}
	component, ok := scene.GetComponent("LocationProxyComponent")
	if !ok || component == nil {
		return ErrLocationProxyMissing
	}
	proxy, ok := component.(interface {
		Get(locationType int, key int64) (actor.ActorID, error)
	})
	if !ok {
		return ErrLocationProxyMissing
	}
	actorID, err := proxy.Get(int(actorlocation.LocationTypeUnit), playerID)
	if err != nil {
		if !errors.Is(err, actorlocation.ErrLocationNotFound) {
			return err
		}
		actorID = actor.ActorID{}
	}
	if actorID.IsValid() {
		return nil
	}

	mapActorID, err := resolveHomeMapActor(scene)
	if err != nil {
		return err
	}
	component, ok = scene.GetComponent("MessageSender")
	if !ok || component == nil {
		return ErrMessageSenderMissing
	}
	sender, ok := component.(interface {
		Call(ctx context.Context, actorID actor.ActorID, msgID uint16, payload []byte) ([]byte, error)
	})
	if !ok {
		return ErrMessageSenderMissing
	}
	payload, err := maprpc.MarshalG2MEnterMap(&maprpc.G2MEnterMap{
		RpcID:    1,
		PlayerID: playerID,
	})
	if err != nil {
		return err
	}
	if callContext == nil {
		callContext = context.Background()
	}
	respPayload, err := sender.Call(callContext, mapActorID, maprpc.MsgG2MEnterMap, payload)
	if err != nil {
		return err
	}
	if len(respPayload) == 0 {
		return errors.New("login: empty map enter response")
	}
	resp, err := maprpc.UnmarshalM2GEnterMap(respPayload)
	if err != nil {
		return err
	}
	if resp.Error != 0 {
		return errors.New(resp.Message)
	}
	actorID, err = proxy.Get(int(actorlocation.LocationTypeUnit), playerID)
	if err != nil {
		return err
	}
	if !actorID.IsValid() {
		return errors.New("login: map enter completed without unit location")
	}
	return nil
}

func resolveHomeMapActor(scene *ecs.Scene) (actor.ActorID, error) {
	if scene == nil {
		return actor.ActorID{}, ErrMapActorMissing
	}
	if actorID, ok := actor.ResolveSceneActor(scene.Zone(), ecs.SceneTypeMap, "Home"); ok {
		return actorID, nil
	}
	return actor.ActorID{}, ErrMapActorMissing
}
