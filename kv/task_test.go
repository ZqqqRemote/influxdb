package kv_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/google/go-cmp/cmp"
	"github.com/influxdata/influxdb/v2"
	icontext "github.com/influxdata/influxdb/v2/context"
	"github.com/influxdata/influxdb/v2/kit/feature"
	"github.com/influxdata/influxdb/v2/kv"
	"github.com/influxdata/influxdb/v2/mock"
	_ "github.com/influxdata/influxdb/v2/query/builtin"
	"github.com/influxdata/influxdb/v2/query/fluxlang"
	"github.com/influxdata/influxdb/v2/task/options"
	"github.com/influxdata/influxdb/v2/task/servicetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestBoltTaskService(t *testing.T) {
	servicetest.TestTaskService(
		t,
		func(t *testing.T) (*servicetest.System, context.CancelFunc) {
			store, close, err := NewTestBoltStore(t)
			if err != nil {
				t.Fatal(err)
			}

			service := kv.NewService(zaptest.NewLogger(t), store, kv.ServiceConfig{
				FluxLanguageService: fluxlang.DefaultService,
			})
			ctx, cancelFunc := context.WithCancel(context.Background())
			if err := service.Initialize(ctx); err != nil {
				t.Fatalf("error initializing urm service: %v", err)
			}

			go func() {
				<-ctx.Done()
				close()
			}()

			return &servicetest.System{
				TaskControlService: service,
				TaskService:        service,
				I:                  service,
				Ctx:                ctx,
			}, cancelFunc
		},
		"transactional",
	)
}

type testService struct {
	Store   kv.Store
	Service *kv.Service
	Org     influxdb.Organization
	User    influxdb.User
	Auth    influxdb.Authorization
	Clock   clock.Clock

	storeCloseFn func()
}

func (s *testService) Close() {
	s.storeCloseFn()
}

func newService(t *testing.T, ctx context.Context, c clock.Clock) *testService {
	t.Helper()

	if c == nil {
		c = clock.New()
	}

	ts := &testService{}
	var err error
	ts.Store, ts.storeCloseFn, err = NewTestInmemStore(t)
	if err != nil {
		t.Fatal("failed to create InmemStore", err)
	}

	ts.Service = kv.NewService(zaptest.NewLogger(t), ts.Store, kv.ServiceConfig{
		Clock:               c,
		FluxLanguageService: fluxlang.DefaultService,
	})
	err = ts.Service.Initialize(ctx)
	if err != nil {
		t.Fatal("Service.Initialize", err)
	}

	ts.User = influxdb.User{Name: t.Name() + "-user"}
	if err := ts.Service.CreateUser(ctx, &ts.User); err != nil {
		t.Fatal(err)
	}
	ts.Org = influxdb.Organization{Name: t.Name() + "-org"}
	if err := ts.Service.CreateOrganization(ctx, &ts.Org); err != nil {
		t.Fatal(err)
	}

	if err := ts.Service.CreateUserResourceMapping(ctx, &influxdb.UserResourceMapping{
		ResourceType: influxdb.OrgsResourceType,
		ResourceID:   ts.Org.ID,
		UserID:       ts.User.ID,
		UserType:     influxdb.Owner,
	}); err != nil {
		t.Fatal(err)
	}

	ts.Auth = influxdb.Authorization{
		OrgID:       ts.Org.ID,
		UserID:      ts.User.ID,
		Permissions: influxdb.OperPermissions(),
	}
	if err := ts.Service.CreateAuthorization(context.Background(), &ts.Auth); err != nil {
		t.Fatal(err)
	}

	return ts
}

func TestRetrieveTaskWithBadAuth(t *testing.T) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	ts := newService(t, ctx, nil)
	defer ts.Close()

	ctx = icontext.SetAuthorizer(ctx, &ts.Auth)

	task, err := ts.Service.CreateTask(ctx, influxdb.TaskCreate{
		Flux:           `option task = {name: "a task",every: 1h} from(bucket:"test") |> range(start:-1h)`,
		OrganizationID: ts.Org.ID,
		OwnerID:        ts.User.ID,
		Status:         string(influxdb.TaskActive),
	})
	if err != nil {
		t.Fatal(err)
	}

	// convert task to old one with a bad auth
	err = ts.Store.Update(ctx, func(tx kv.Tx) error {
		b, err := tx.Bucket([]byte("tasksv1"))
		if err != nil {
			return err
		}
		bID, err := task.ID.Encode()
		if err != nil {
			return err
		}
		task.OwnerID = influxdb.ID(1)
		task.AuthorizationID = influxdb.ID(132) // bad id or an id that doesnt match any auth
		tbyte, err := json.Marshal(task)
		if err != nil {
			return err
		}
		// have to actually hack the bytes here because the system doesnt like us to encode bad id's.
		tbyte = bytes.Replace(tbyte, []byte(`,"ownerID":"0000000000000001"`), []byte{}, 1)
		if err := b.Put(bID, tbyte); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// lets see if we can list and find the task
	newTask, err := ts.Service.FindTaskByID(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if newTask.ID != task.ID {
		t.Fatal("miss matching taskID's")
	}

	tasks, _, err := ts.Service.FindTasks(context.Background(), influxdb.TaskFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatal("failed to return task")
	}

	// test status filter
	active := string(influxdb.TaskActive)
	tasksWithActiveFilter, _, err := ts.Service.FindTasks(context.Background(), influxdb.TaskFilter{Status: &active})
	if err != nil {
		t.Fatal("could not find tasks")
	}
	if len(tasksWithActiveFilter) != 1 {
		t.Fatal("failed to find active task with filter")
	}
}

func TestService_UpdateTask_InactiveToActive(t *testing.T) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	c := clock.NewMock()
	c.Set(time.Unix(1000, 0))

	ts := newService(t, ctx, c)
	defer ts.Close()

	ctx = icontext.SetAuthorizer(ctx, &ts.Auth)

	originalTask, err := ts.Service.CreateTask(ctx, influxdb.TaskCreate{
		Flux:           `option task = {name: "a task",every: 1h} from(bucket:"test") |> range(start:-1h)`,
		OrganizationID: ts.Org.ID,
		OwnerID:        ts.User.ID,
		Status:         string(influxdb.TaskActive),
	})
	if err != nil {
		t.Fatal("CreateTask", err)
	}

	v := influxdb.TaskStatusInactive
	c.Add(1 * time.Second)
	exp := c.Now()
	updatedTask, err := ts.Service.UpdateTask(ctx, originalTask.ID, influxdb.TaskUpdate{Status: &v, LatestCompleted: &exp, LatestScheduled: &exp})
	if err != nil {
		t.Fatal("UpdateTask", err)
	}

	if got := updatedTask.LatestScheduled; !got.Equal(exp) {
		t.Fatalf("unexpected -got/+exp\n%s", cmp.Diff(got.String(), exp.String()))
	}
	if got := updatedTask.LatestCompleted; !got.Equal(exp) {
		t.Fatalf("unexpected -got/+exp\n%s", cmp.Diff(got.String(), exp.String()))
	}

	c.Add(10 * time.Second)
	exp = c.Now()
	v = influxdb.TaskStatusActive
	updatedTask, err = ts.Service.UpdateTask(ctx, originalTask.ID, influxdb.TaskUpdate{Status: &v})
	if err != nil {
		t.Fatal("UpdateTask", err)
	}

	if got := updatedTask.LatestScheduled; !got.Equal(exp) {
		t.Fatalf("unexpected -got/+exp\n%s", cmp.Diff(got.String(), exp.String()))
	}
}

func TestTaskRunCancellation(t *testing.T) {
	store, close, err := NewTestBoltStore(t)
	if err != nil {
		t.Fatal(err)
	}
	defer close()

	service := kv.NewService(zaptest.NewLogger(t), store, kv.ServiceConfig{
		FluxLanguageService: fluxlang.DefaultService,
	})
	ctx, cancelFunc := context.WithCancel(context.Background())
	if err := service.Initialize(ctx); err != nil {
		t.Fatalf("error initializing urm service: %v", err)
	}
	defer cancelFunc()
	u := &influxdb.User{Name: t.Name() + "-user"}
	if err := service.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	o := &influxdb.Organization{Name: t.Name() + "-org"}
	if err := service.CreateOrganization(ctx, o); err != nil {
		t.Fatal(err)
	}

	if err := service.CreateUserResourceMapping(ctx, &influxdb.UserResourceMapping{
		ResourceType: influxdb.OrgsResourceType,
		ResourceID:   o.ID,
		UserID:       u.ID,
		UserType:     influxdb.Owner,
	}); err != nil {
		t.Fatal(err)
	}

	authz := influxdb.Authorization{
		OrgID:       o.ID,
		UserID:      u.ID,
		Permissions: influxdb.OperPermissions(),
	}
	if err := service.CreateAuthorization(context.Background(), &authz); err != nil {
		t.Fatal(err)
	}

	ctx = icontext.SetAuthorizer(ctx, &authz)

	task, err := service.CreateTask(ctx, influxdb.TaskCreate{
		Flux:           `option task = {name: "a task",cron: "0 * * * *", offset: 20s} from(bucket:"test") |> range(start:-1h)`,
		OrganizationID: o.ID,
		OwnerID:        u.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	run, err := service.CreateRun(ctx, task.ID, time.Now().Add(time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	if err := service.CancelRun(ctx, run.TaskID, run.ID); err != nil {
		t.Fatal(err)
	}

	canceled, err := service.FindRunByID(ctx, run.TaskID, run.ID)
	if err != nil {
		t.Fatal(err)
	}

	if canceled.Status != influxdb.RunCanceled.String() {
		t.Fatalf("expected task run to be cancelled")
	}
}

func TestTaskMigrate(t *testing.T) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	ts := newService(t, ctx, nil)
	defer ts.Close()

	id := "05da585043e02000"
	// create a task that has auth set and no ownerID
	err := ts.Store.Update(context.Background(), func(tx kv.Tx) error {
		b, err := tx.Bucket([]byte("tasksv1"))
		if err != nil {
			t.Fatal(err)
		}
		taskBody := fmt.Sprintf(`{"id":"05da585043e02000","type":"system","orgID":"05d3ae3492c9c000","org":"whos","authorizationID":"%s","name":"asdf","status":"active","flux":"option v = {\n  bucket: \"bucks\",\n  timeRangeStart: -1h,\n  timeRangeStop: now()\n}\n\noption task = { \n  name: \"asdf\",\n  every: 5m,\n}\n\nfrom(bucket: \"_monitoring\")\n  |\u003e range(start: v.timeRangeStart, stop: v.timeRangeStop)\n  |\u003e filter(fn: (r) =\u003e r[\"_measurement\"] == \"boltdb_reads_total\")\n  |\u003e filter(fn: (r) =\u003e r[\"_field\"] == \"counter\")\n  |\u003e to(bucket: \"bucks\", org: \"whos\")","every":"5m","latestCompleted":"2020-06-16T17:01:26.083319Z","latestScheduled":"2020-06-16T17:01:26.083319Z","lastRunStatus":"success","createdAt":"2020-06-15T19:10:29Z","updatedAt":"0001-01-01T00:00:00Z"}`, ts.Auth.ID.String())
		err = b.Put([]byte(id), []byte(taskBody))

		if err != nil {
			t.Fatal(err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = ts.Service.TaskOwnerIDUpMigration(context.Background(), ts.Store)
	if err != nil {
		t.Fatal(err)
	}
	idType, _ := influxdb.IDFromString(id)
	task, err := ts.Service.FindTaskByID(context.Background(), *idType)
	if err != nil {
		t.Fatal(err)
	}
	if task.OwnerID != ts.User.ID {
		t.Fatal("failed to fill in ownerID")
	}

	// create a task that has no auth or owner id but a urm exists
	err = ts.Store.Update(context.Background(), func(tx kv.Tx) error {
		b, err := tx.Bucket([]byte("tasksv1"))
		if err != nil {
			t.Fatal(err)
		}
		taskBody := fmt.Sprintf(`{"id":"05da585043e02000","type":"system","orgID":"%s","org":"whos","name":"asdf","status":"active","flux":"option v = {\n  bucket: \"bucks\",\n  timeRangeStart: -1h,\n  timeRangeStop: now()\n}\n\noption task = { \n  name: \"asdf\",\n  every: 5m,\n}\n\nfrom(bucket: \"_monitoring\")\n  |\u003e range(start: v.timeRangeStart, stop: v.timeRangeStop)\n  |\u003e filter(fn: (r) =\u003e r[\"_measurement\"] == \"boltdb_reads_total\")\n  |\u003e filter(fn: (r) =\u003e r[\"_field\"] == \"counter\")\n  |\u003e to(bucket: \"bucks\", org: \"whos\")","every":"5m","latestCompleted":"2020-06-16T17:01:26.083319Z","latestScheduled":"2020-06-16T17:01:26.083319Z","lastRunStatus":"success","createdAt":"2020-06-15T19:10:29Z","updatedAt":"0001-01-01T00:00:00Z"}`, ts.Org.ID.String())
		err = b.Put([]byte(id), []byte(taskBody))
		if err != nil {
			t.Fatal(err)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = ts.Service.TaskOwnerIDUpMigration(context.Background(), ts.Store)
	if err != nil {
		t.Fatal(err)
	}

	task, err = ts.Service.FindTaskByID(context.Background(), *idType)
	if err != nil {
		t.Fatal(err)
	}
	if task.OwnerID != ts.User.ID {
		t.Fatal("failed to fill in ownerID")
	}
}

type taskOptions struct {
	name        string
	every       string
	cron        string
	offset      string
	concurrency int64
	retry       int64
}

func TestExtractTaskOptions(t *testing.T) {
	tcs := []struct {
		name     string
		flux     string
		expected taskOptions
		errMsg   string
	}{
		{
			name: "all parameters",
			flux: `option task = {name: "whatever", every: 1s, offset: 0s, concurrency: 2, retry: 2}`,
			expected: taskOptions{
				name:        "whatever",
				every:       "1s",
				offset:      "0s",
				concurrency: 2,
				retry:       2,
			},
		},
		{
			name: "some extra whitespace and bad content around it",
			flux: `howdy()
			option     task    =     { name:"whatever",  cron:  "* * * * *"  }
			hello()
			`,
			expected: taskOptions{
				name:        "whatever",
				cron:        "* * * * *",
				concurrency: 1,
				retry:       1,
			},
		},
		{
			name:   "bad options",
			flux:   `option task = {name: "whatever", every: 1s, cron: "* * * * *"}`,
			errMsg: "cannot use both cron and every in task options",
		},
		{
			name:   "no options",
			flux:   `doesntexist()`,
			errMsg: "no task options defined",
		},
		{
			name: "multiple assignments",
			flux: `
			option task = {name: "whatever", every: 1s, offset: 0s, concurrency: 2, retry: 2}
			option task = {name: "whatever", every: 1s, offset: 0s, concurrency: 2, retry: 2}
			`,
			errMsg: "multiple task options defined",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			flagger := mock.NewFlagger(map[feature.Flag]interface{}{
				feature.SimpleTaskOptionsExtraction(): true,
			})
			ctx, _ := feature.Annotate(context.Background(), flagger)
			opts, err := kv.ExtractTaskOptions(ctx, fluxlang.DefaultService, tc.flux)
			if tc.errMsg != "" {
				require.Error(t, err)
				assert.Equal(t, tc.errMsg, err.Error())
				return
			}

			require.NoError(t, err)

			var offset options.Duration
			if opts.Offset != nil {
				offset = *opts.Offset
			}

			var concur int64
			if opts.Concurrency != nil {
				concur = *opts.Concurrency
			}

			var retry int64
			if opts.Retry != nil {
				retry = *opts.Retry
			}

			assert.Equal(t, tc.expected.name, opts.Name)
			assert.Equal(t, tc.expected.cron, opts.Cron)
			assert.Equal(t, tc.expected.every, opts.Every.String())
			assert.Equal(t, tc.expected.offset, offset.String())
			assert.Equal(t, tc.expected.concurrency, concur)
			assert.Equal(t, tc.expected.retry, retry)
		})
	}
}
