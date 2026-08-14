package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	mongooptions "go.mongodb.org/mongo-driver/mongo/options"
)

const migrationCollectionName = "schema_migrations"
const migrationLockCollectionName = "schema_migration_locks"
const migrationLockID = "global"
const defaultMigrationLeaseDuration = 5 * time.Minute
const defaultMigrationPollInterval = 100 * time.Millisecond
const migrationReleaseTimeout = 5 * time.Second

var (
	// ErrMigrationContextRequired 表示 migration runner 缺少上下文。
	ErrMigrationContextRequired = errors.New("db: migration context required")
	// ErrMigrationClientRequired 表示 migration runner 缺少已连接客户端。
	ErrMigrationClientRequired = errors.New("db: migration client required")
	// ErrMigrationInvalid 表示 migration 定义不完整或版本非法。
	ErrMigrationInvalid = errors.New("db: invalid migration")
	// ErrMigrationVersionConflict 表示同一版本已经使用了不同名称。
	ErrMigrationVersionConflict = errors.New("db: migration version conflict")
	// ErrMigrationOptionsInvalid 表示 migration 锁租约参数非法。
	ErrMigrationOptionsInvalid = errors.New("db: invalid migration options")
	// ErrMigrationLockLost 表示 migration 执行期间失去 MongoDB 租约。
	ErrMigrationLockLost = errors.New("db: migration lock lost")
)

// Migration 描述一个有序的数据库变更。
//
// Version 必须从正数开始且唯一。Up 必须幂等，便于进程在写入
// schema_migrations 前崩溃后安全重试。
type Migration struct {
	Version int32
	Name    string
	Up      func(context.Context, *Client) error
}

// MigrationOptions 控制 migration 的分布式锁。
//
// Owner 为空时由当前调用生成随机 owner。LeaseDuration 是 MongoDB 锁租约
// 时间，migration 执行期间由心跳续租；PollInterval 是获取锁时的重试间隔。
type MigrationOptions struct {
	Owner         string
	LeaseDuration time.Duration
	PollInterval  time.Duration
}

type appliedMigration struct {
	Version   int32     `bson:"_id"`
	Name      string    `bson:"name"`
	AppliedAt time.Time `bson:"applied_at"`
}

type migrationLease struct {
	collection *mongo.Collection
	owner      string
	duration   time.Duration
	stop       chan struct{}
	done       chan struct{}
	stopOnce   sync.Once
	cancel     context.CancelFunc
	errMu      sync.RWMutex
	leaseError error
}

// ValidateMigrations 校验 migration 的版本和名称。
func ValidateMigrations(migrations []Migration) error {
	seen := make(map[int32]struct{}, len(migrations))
	for _, migration := range migrations {
		if migration.Version <= 0 || migration.Name == "" || migration.Up == nil {
			return fmt.Errorf("%w: version=%d name=%q", ErrMigrationInvalid, migration.Version, migration.Name)
		}
		if _, exists := seen[migration.Version]; exists {
			return fmt.Errorf("%w: duplicate version=%d", ErrMigrationInvalid, migration.Version)
		}
		seen[migration.Version] = struct{}{}
	}
	return nil
}

// RunMigrations 按版本顺序执行尚未应用的 migration。
func RunMigrations(ctx context.Context, client *Client, migrations []Migration) error {
	return RunMigrationsWithOptions(ctx, client, migrations, MigrationOptions{})
}

// RunMigrationsWithOptions 按版本顺序执行 migration，并使用 MongoDB 租约
// 锁协调多个 Process。
func RunMigrationsWithOptions(ctx context.Context, client *Client, migrations []Migration, options MigrationOptions) (err error) {
	if ctx == nil {
		return ErrMigrationContextRequired
	}
	if client == nil || client.database == nil {
		return ErrMigrationClientRequired
	}
	if err := ValidateMigrations(migrations); err != nil {
		return err
	}
	if len(migrations) == 0 {
		return nil
	}

	normalized, err := normalizeMigrationOptions(options)
	if err != nil {
		return err
	}
	lockCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	lease, err := acquireMigrationLease(lockCtx, cancel, client, normalized)
	if err != nil {
		return err
	}
	defer func() {
		releaseErr := lease.Release()
		if releaseErr == nil {
			return
		}
		if err == nil {
			err = releaseErr
			return
		}
		err = errors.Join(err, releaseErr)
	}()

	ordered := append([]Migration(nil), migrations...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Version < ordered[j].Version
	})

	collection := client.database.Collection(migrationCollectionName)
	for _, migration := range ordered {
		if leaseErr := lease.Err(); leaseErr != nil {
			return leaseErr
		}
		var applied appliedMigration
		err := collection.FindOne(lockCtx, bson.M{"_id": migration.Version}).Decode(&applied)
		switch {
		case err == nil:
			if applied.Name != migration.Name {
				return fmt.Errorf("%w: version=%d applied=%q configured=%q",
					ErrMigrationVersionConflict,
					migration.Version,
					applied.Name,
					migration.Name,
				)
			}
			continue
		case errors.Is(err, mongo.ErrNoDocuments):
			// 继续执行新的 migration。
		default:
			return fmt.Errorf("db: read migration version %d: %w", migration.Version, err)
		}

		if err := migration.Up(lockCtx, client); err != nil {
			return fmt.Errorf("db: apply migration %d (%s): %w", migration.Version, migration.Name, err)
		}
		if leaseErr := lease.Err(); leaseErr != nil {
			return leaseErr
		}
		if _, err := collection.InsertOne(lockCtx, appliedMigration{
			Version:   migration.Version,
			Name:      migration.Name,
			AppliedAt: time.Now().UTC(),
		}); err != nil {
			return fmt.Errorf("db: record migration %d (%s): %w", migration.Version, migration.Name, err)
		}
	}
	return nil
}

func normalizeMigrationOptions(options MigrationOptions) (MigrationOptions, error) {
	options.Owner = strings.TrimSpace(options.Owner)
	if options.Owner == "" {
		owner, err := newMigrationOwner()
		if err != nil {
			return MigrationOptions{}, err
		}
		options.Owner = owner
	}
	if options.LeaseDuration <= 0 {
		options.LeaseDuration = defaultMigrationLeaseDuration
	}
	if options.PollInterval <= 0 {
		options.PollInterval = defaultMigrationPollInterval
	}
	if options.PollInterval >= options.LeaseDuration {
		return MigrationOptions{}, fmt.Errorf("%w: poll interval %s must be shorter than lease %s",
			ErrMigrationOptionsInvalid,
			options.PollInterval,
			options.LeaseDuration,
		)
	}
	return options, nil
}

func newMigrationOwner() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("db: generate migration owner: %w", err)
	}
	return "migration-" + hex.EncodeToString(buf), nil
}

func acquireMigrationLease(ctx context.Context, cancel context.CancelFunc, client *Client, options MigrationOptions) (*migrationLease, error) {
	if ctx == nil {
		return nil, ErrMigrationContextRequired
	}
	if client == nil || client.database == nil {
		return nil, ErrMigrationClientRequired
	}
	collection := client.database.Collection(migrationLockCollectionName)
	for {
		now := time.Now().UTC()
		result, err := collection.UpdateOne(
			ctx,
			bson.M{
				"_id": migrationLockID,
				"$or": bson.A{
					bson.M{"owner": options.Owner},
					bson.M{"expires_at": bson.M{"$lte": now}},
					bson.M{"expires_at": bson.M{"$exists": false}},
				},
			},
			bson.M{
				"$set": bson.M{
					"owner":       options.Owner,
					"acquired_at": now,
					"expires_at":  now.Add(options.LeaseDuration),
				},
				"$setOnInsert": bson.M{"_id": migrationLockID},
			},
			mongooptions.Update().SetUpsert(true),
		)
		if err == nil && (result.MatchedCount == 1 || result.UpsertedCount == 1) {
			lease := &migrationLease{
				collection: collection,
				owner:      options.Owner,
				duration:   options.LeaseDuration,
				stop:       make(chan struct{}),
				done:       make(chan struct{}),
				cancel:     cancel,
			}
			go lease.heartbeat(ctx)
			return lease, nil
		}
		if err != nil && !mongo.IsDuplicateKeyError(err) {
			return nil, fmt.Errorf("db: acquire migration lock: %w", err)
		}
		timer := time.NewTimer(options.PollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (l *migrationLease) heartbeat(ctx context.Context) {
	defer close(l.done)
	interval := l.duration / 3
	if interval <= 0 {
		interval = time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-l.stop:
			return
		case now := <-ticker.C:
			renewCtx, cancel := context.WithTimeout(ctx, interval)
			result, err := l.collection.UpdateOne(
				renewCtx,
				bson.M{"_id": migrationLockID, "owner": l.owner},
				bson.M{"$set": bson.M{"expires_at": now.UTC().Add(l.duration)}},
			)
			cancel()
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				l.setError(fmt.Errorf("%w: renew migration lock: %v", ErrMigrationLockLost, err))
				if l.cancel != nil {
					l.cancel()
				}
				return
			}
			if result.MatchedCount != 1 {
				l.setError(ErrMigrationLockLost)
				if l.cancel != nil {
					l.cancel()
				}
				return
			}
		}
	}
}

func (l *migrationLease) setError(err error) {
	if l == nil || err == nil {
		return
	}
	l.errMu.Lock()
	if l.leaseError == nil {
		l.leaseError = err
	}
	l.errMu.Unlock()
}

func (l *migrationLease) Err() error {
	if l == nil {
		return ErrMigrationLockLost
	}
	l.errMu.RLock()
	defer l.errMu.RUnlock()
	return l.leaseError
}

func (l *migrationLease) Release() error {
	if l == nil {
		return nil
	}
	l.stopOnce.Do(func() {
		close(l.stop)
	})
	<-l.done

	ctx, cancel := context.WithTimeout(context.Background(), migrationReleaseTimeout)
	defer cancel()
	result, err := l.collection.UpdateOne(
		ctx,
		bson.M{"_id": migrationLockID, "owner": l.owner},
		bson.M{"$set": bson.M{
			"owner":      "",
			"expires_at": time.Now().UTC(),
		}},
	)
	if err != nil {
		return errors.Join(l.Err(), fmt.Errorf("db: release migration lock: %w", err))
	}
	if result.MatchedCount != 1 {
		return errors.Join(l.Err(), ErrMigrationLockLost)
	}
	return l.Err()
}
