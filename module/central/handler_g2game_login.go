package central

import (
	"context"
	"fmt"
	"time"

	"github.com/jerbe/et-go/db"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/gamelogin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// HandleGameLogin 查询或创建玩家档案，返回 PlayerId。
func HandleGameLogin(scene *ecs.Scene, req *gamelogin.G2GameLogin) (*gamelogin.Game2GLogin, error) {
	if req == nil {
		return nil, ErrSceneMissing
	}
	if scene == nil {
		return nil, ErrSceneMissing
	}
	if req.AccountId <= 0 {
		return nil, ErrPlayerProfileInvalid
	}

	profile, err := loadOrCreatePlayerProfile(scene, req.AccountId)
	if err != nil {
		return nil, err
	}
	if profile == nil || profile.Id <= 0 {
		return nil, ErrPlayerProfileInvalid
	}

	return &gamelogin.Game2GLogin{
		RpcId:     req.RpcId,
		AccountId: req.AccountId,
		ZoneId:    int32(scene.Zone()),
		PlayerId:  profile.Id,
	}, nil
}

func loadOrCreatePlayerProfile(scene *ecs.Scene, accountId int64) (*db.CPlayerProfile, error) {
	if scene == nil {
		return nil, ErrSceneMissing
	}
	if component, ok := scene.GetComponent("PlayerProfileStoreComponent"); ok && component != nil {
		provider, ok := component.(*PlayerProfileStoreComponent)
		if !ok || provider.Store() == nil {
			return nil, ErrPlayerProfileStoreMissing
		}
		return provider.Store().LoadOrCreatePlayerProfile(context.Background(), scene.Zone(), accountId)
	}

	component, ok := scene.GetComponent("DBManagerComponent")
	if !ok || component == nil {
		return nil, ErrPlayerProfileStoreMissing
	}
	manager, ok := component.(interface {
		GetZoneDB(zone int) (*db.DBComponent, error)
	})
	if !ok {
		return nil, ErrPlayerProfileStoreMissing
	}
	zoneDB, err := manager.GetZoneDB(scene.Zone())
	if err != nil {
		return nil, err
	}
	var profiles []db.CPlayerProfile
	if err := zoneDB.Query(context.Background(), bson.M{
		"account_id": accountId,
		"zone_id":    int32(scene.Zone()),
	}, (&db.CPlayerProfile{}).CollectionName(), &profiles); err != nil {
		return nil, err
	}
	if profile, err := uniquePlayerProfile(profiles, accountId, scene.Zone()); err != nil || profile != nil {
		return profile, err
	}
	id, err := zoneDB.Increment(context.Background(), "player_id", 1, 1000000)
	if err != nil {
		return nil, err
	}
	profile := &db.CPlayerProfile{
		Id:        id,
		ZoneId:    int32(scene.Zone()),
		AccountId: accountId,
		ShortId:   fmt.Sprintf("%d", id),
		CreatedAt: time.Now(),
	}
	if err := zoneDB.Insert(context.Background(), profile, profile.CollectionName()); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			var existing []db.CPlayerProfile
			if queryErr := zoneDB.Query(context.Background(), bson.M{
				"account_id": accountId,
				"zone_id":    int32(scene.Zone()),
			}, (&db.CPlayerProfile{}).CollectionName(), &existing); queryErr != nil {
				return nil, queryErr
			}
			if profile, selectErr := uniquePlayerProfile(existing, accountId, scene.Zone()); selectErr != nil || profile != nil {
				return profile, selectErr
			}
		}
		return nil, err
	}
	return profile, nil
}

func uniquePlayerProfile(profiles []db.CPlayerProfile, accountID int64, zone int) (*db.CPlayerProfile, error) {
	switch len(profiles) {
	case 0:
		return nil, nil
	case 1:
		profile := profiles[0]
		return &profile, nil
	default:
		return nil, fmt.Errorf("%w: account_id=%d zone_id=%d count=%d",
			ErrPlayerProfileDuplicate,
			accountID,
			zone,
			len(profiles),
		)
	}
}
