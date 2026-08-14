package http

import (
	"context"
	"errors"

	"github.com/jerbe/et-go/config"
	"github.com/jerbe/et-go/db"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/central"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var (
	// ErrUsernameAlreadyRegistered 表示用户名已存在。
	ErrUsernameAlreadyRegistered = errors.New("http: username already registered")
	// ErrAccountRepositoryMissing 表示账号仓储未配置。
	ErrAccountRepositoryMissing = errors.New("http: account repository missing")
)

// AccountRepository 定义 HTTP 账号读写能力。
type AccountRepository interface {
	FindByUsername(ctx context.Context, username string) (*central.CAccount, error)
	CreateAccount(ctx context.Context, username string, passwordHash string, algorithm string) (int64, error)
	UpdatePassword(ctx context.Context, accountID int64, passwordHash string, algorithm string) error
}

type dbAccountRepository struct {
	component *db.DBComponent
	centralDB *db.DBComponent
}

func (r *dbAccountRepository) FindByUsername(ctx context.Context, username string) (*central.CAccount, error) {
	var accounts []db.CAccount
	if err := r.component.Query(ctx, bson.M{"username": username}, (&db.CAccount{}).CollectionName(), &accounts); err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, nil
	}
	if len(accounts) > 1 {
		return nil, central.ErrAccountDuplicate
	}
	account := accounts[0]
	return &account, nil
}

func (r *dbAccountRepository) CreateAccount(ctx context.Context, username string, passwordHash string, algorithm string) (int64, error) {
	account, err := r.FindByUsername(ctx, username)
	if err != nil {
		return 0, err
	}
	if account != nil {
		return 0, ErrUsernameAlreadyRegistered
	}
	if r.centralDB == nil {
		return 0, ErrAccountRepositoryMissing
	}
	accountID, err := r.centralDB.Increment(ctx, "account_id", 1, 1000000)
	if err != nil {
		return 0, err
	}
	entity := &db.CAccount{
		Id:                accountID,
		Username:          username,
		PasswordHash:      passwordHash,
		PasswordAlgorithm: algorithm,
	}
	if err := r.component.Insert(ctx, entity, entity.CollectionName()); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return 0, ErrUsernameAlreadyRegistered
		}
		return 0, err
	}
	return accountID, nil
}

func (r *dbAccountRepository) UpdatePassword(ctx context.Context, accountID int64, passwordHash string, algorithm string) error {
	if accountID <= 0 || passwordHash == "" || algorithm == "" {
		return central.ErrPasswordHashInvalid
	}
	var account db.CAccount
	if err := r.component.FindOne(ctx, accountID, (&db.CAccount{}).CollectionName(), &account); err != nil {
		return err
	}
	account.PasswordHash = passwordHash
	account.PasswordAlgorithm = algorithm
	return r.component.Save(ctx, accountID, &account, account.CollectionName())
}

func accountRepositoryFromScene(scene *ecs.Scene) (AccountRepository, error) {
	if scene == nil {
		return nil, ErrAccountRepositoryMissing
	}
	if component, ok := scene.GetComponent("HTTPAccountRepositoryComponent"); ok && component != nil {
		provider, ok := component.(interface {
			Repository() AccountRepository
		})
		if ok && provider.Repository() != nil {
			return provider.Repository(), nil
		}
	}
	component, ok := scene.GetComponent("DBManagerComponent")
	if !ok || component == nil {
		return nil, ErrAccountRepositoryMissing
	}
	manager, ok := component.(interface {
		GetZoneDB(zone int) (*db.DBComponent, error)
	})
	if !ok {
		return nil, ErrAccountRepositoryMissing
	}
	zone, err := resolveDBZone(scene)
	if err != nil {
		return nil, err
	}
	zoneDB, err := manager.GetZoneDB(zone)
	if err != nil {
		return nil, err
	}
	centralZone, err := resolveCentralZone()
	if err != nil {
		return nil, err
	}
	centralDB, err := manager.GetZoneDB(centralZone)
	if err != nil {
		return nil, err
	}
	return &dbAccountRepository{component: zoneDB, centralDB: centralDB}, nil
}

func resolveDBZone(scene *ecs.Scene) (int, error) {
	if scene == nil || scene.Zone() <= 0 {
		return 0, ErrAccountRepositoryMissing
	}
	return scene.Zone(), nil
}

func resolveCentralZone() (int, error) {
	cfg := config.GetGlobal()
	if cfg == nil {
		return 0, ErrAccountRepositoryMissing
	}
	for _, zoneCfg := range cfg.Zones {
		if zoneCfg.ID == 1 {
			return zoneCfg.ID, nil
		}
	}
	return 0, ErrAccountRepositoryMissing
}
