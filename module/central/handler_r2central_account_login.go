package central

import (
	"context"
	"fmt"

	"github.com/jerbe/et-go/db"
	"github.com/jerbe/et-go/engine/ecs"
	"github.com/jerbe/et-go/module/login"
	"go.mongodb.org/mongo-driver/bson"
)

// AccountStore 定义 Central 账号查询能力。
type AccountStore interface {
	FindByUsername(ctx context.Context, username string) (*CAccount, error)
	UpdatePassword(ctx context.Context, accountID int64, passwordHash string, algorithm string) error
}

type dbAccountStore struct {
	component *db.DBComponent
}

func (s *dbAccountStore) FindByUsername(ctx context.Context, username string) (*CAccount, error) {
	var accounts []db.CAccount
	if err := s.component.Query(ctx, bson.M{
		"username": username,
	}, (&db.CAccount{}).CollectionName(), &accounts); err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, nil
	}
	if len(accounts) > 1 {
		return nil, ErrAccountDuplicate
	}
	account := accounts[0]
	return &account, nil
}

func (s *dbAccountStore) UpdatePassword(ctx context.Context, accountID int64, passwordHash string, algorithm string) error {
	if accountID <= 0 || passwordHash == "" || algorithm == "" {
		return ErrPasswordHashInvalid
	}
	var account db.CAccount
	if err := s.component.FindOne(ctx, accountID, (&db.CAccount{}).CollectionName(), &account); err != nil {
		return err
	}
	account.PasswordHash = passwordHash
	account.PasswordAlgorithm = algorithm
	return s.component.Save(ctx, accountID, &account, account.CollectionName())
}

// HandleAccountLoginWithStore 使用指定存储执行账号校验。
func HandleAccountLoginWithStore(store AccountStore, req *R2CentralAccountLogin) (*Central2RAccountLogin, error) {
	if req == nil {
		return nil, ErrInvalidAccountLoginRequest
	}
	if store == nil {
		return nil, ErrInvalidAccountLoginRequest
	}
	account, err := store.FindByUsername(context.Background(), req.Username)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return &Central2RAccountLogin{
			RpcId:   req.RpcId,
			Error:   ERR_UsernameOrPasswordIncorrectError,
			Message: ErrUsernameOrPasswordIncorrect.Error(),
		}, nil
	}
	valid, needsUpgrade, err := VerifyPassword(req.Password, account.PasswordHash, account.PasswordAlgorithm)
	if err != nil {
		return nil, err
	}
	if !valid {
		return &Central2RAccountLogin{
			RpcId:   req.RpcId,
			Error:   ERR_UsernameOrPasswordIncorrectError,
			Message: ErrUsernameOrPasswordIncorrect.Error(),
		}, nil
	}
	if needsUpgrade {
		passwordHash, err := HashPassword(req.Password)
		if err != nil {
			return nil, fmt.Errorf("%w: hash password: %v", ErrPasswordUpgradeFailed, err)
		}
		if err := store.UpdatePassword(context.Background(), account.Id, passwordHash, PasswordAlgorithmArgon2id); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrPasswordUpgradeFailed, err)
		}
	}
	token, err := login.GenerateAccessToken(account.Id)
	if err != nil {
		return nil, err
	}
	return &Central2RAccountLogin{
		RpcId:       req.RpcId,
		AccessToken: token,
	}, nil
}

// HandleAccountLogin 从场景依赖执行账号校验。
func HandleAccountLogin(scene *ecs.Scene, req *R2CentralAccountLogin) (*Central2RAccountLogin, error) {
	store, err := accountStoreFromScene(scene)
	if err != nil {
		return nil, err
	}
	return HandleAccountLoginWithStore(store, req)
}

func accountStoreFromScene(scene *ecs.Scene) (AccountStore, error) {
	if scene == nil {
		return nil, ErrAccountStoreMissing
	}
	if component, ok := scene.GetComponent("CentralAccountStoreComponent"); ok && component != nil {
		provider, ok := component.(interface {
			Store() AccountStore
		})
		if ok && provider.Store() != nil {
			return provider.Store(), nil
		}
	}
	component, ok := scene.GetComponent("DBManagerComponent")
	if !ok || component == nil {
		return nil, ErrAccountStoreMissing
	}
	manager, ok := component.(interface {
		GetZoneDB(zone int) (*db.DBComponent, error)
	})
	if !ok {
		return nil, ErrAccountStoreMissing
	}
	dbComponent, err := manager.GetZoneDB(scene.Zone())
	if err != nil {
		return nil, err
	}
	return &dbAccountStore{component: dbComponent}, nil
}

func marshalAccountLoginResponse(resp *Central2RAccountLogin) ([]byte, error) {
	return marshalCentral2RAccountLogin(resp)
}
