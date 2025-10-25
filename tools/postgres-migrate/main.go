//go:build e2e_migrate
// +build e2e_migrate

package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgconn"
	v1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"github.com/tektoncd/results/pkg/api/server/db"
	_ "github.com/tektoncd/results/pkg/api/server/db/errors/postgres"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	pb "github.com/tektoncd/results/proto/pipeline/v1/pipeline_go_proto"
)

var (
	write = flag.Bool("write", false, "enable migration writes. if disabled, the tool still prints a summary of what would be migrated.")
)

func main() {
	flag.Parse()

	mysql, err := openMysql()
	if err != nil {
		log.Fatalf("failed to open the mysql db: %v", err)
	}

	postgres, err := openPostgres()
	if err != nil {
		log.Fatalf("failed to open the postgres db: %v", err)
	}

	out, err := migrate(mysql, postgres, *write)
	if err != nil {
		log.Println("failed to migrate:", err)
	}
	if b, err := json.MarshalIndent(out, "", "  "); err == nil {
		fmt.Printf("Outcome:\n%s", string(b))
	} else {
		fmt.Printf("Outcome:\n%+v", out)
	}
}

func openMysql() (*gorm.DB, error) {
	mysqlURI := fmt.Sprintf("%s:%s@%s(%s)/%s?parseTime=true", os.Getenv("MYSQL_USER"), os.Getenv("MYSQL_PASSWORD"), os.Getenv("MYSQL_PROTOCOL"), os.Getenv("MYSQL_ADDR"), os.Getenv("MYSQL_DB"))
	return gorm.Open(mysql.Open(mysqlURI), &gorm.Config{})
}

func openPostgres() (*gorm.DB, error) {
	postgresURI := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s", os.Getenv("POSTGRES_ADDR"), os.Getenv("POSTGRES_USER"), os.Getenv("POSTGRES_PASSWORD"), os.Getenv("POSTGRES_DB"), os.Getenv("POSTGRES_PORT"))
	return gorm.Open(postgres.Open(postgresURI), &gorm.Config{})
}

const (
	// UniqueViolationErr represents Postgres unique_violation errors
	// typically returned if the primary key / indexed key we are trying to
	// write already exists.
	// See https://www.postgresql.org/docs/13/errcodes-appendix.html
	UniqueViolationErrCode = "23505"

	// OutcomeSuccess means that the entity was successfully migrated.
	OutcomeSuccess outcome = "SUCCESS"
	// OutcomeDryRun means that the entity would have been migrated, but was
	// not because dry run mode was enabled.
	OutcomeDryRun outcome = "WRITE_DISABLED"
	// OutcomeAlreadyExists means that the entity was not migrated because an
	// entity with the same name already exists in the new database.
	OutcomeAlreadyExists outcome = "ALREADY_EXISTS"
)

type outcome string
type outcomeLog map[string]outcome

func migrate(mysql, postgres *gorm.DB, write bool) (outcomeLog, error) {
	outcomes := make(outcomeLog)

	var results []*db.Result
	out := mysql.Find(&results)
	if err := out.Error; err != nil {
		return outcomes, fmt.Errorf("error reading MySQL results: %w", err)
	}

	for _, r := range results {
		key := fmt.Sprintf("%s/results/%s", r.Parent, r.Name)

		if write {
			out := postgres.Create(&r)
			if err := out.Error; err != nil {
				// Check if result already exists - see https://github.com/go-gorm/gorm/issues/4135
				var perr *pgconn.PgError
				if errors.As(err, &perr) && perr.Code == UniqueViolationErrCode {
					outcomes[key] = OutcomeAlreadyExists
					continue
				} else {
					return outcomes, fmt.Errorf("error creating postgres result: %w", out.Error)
				}
			}
			outcomes[key] = OutcomeSuccess
		} else {
			// Since this is a dry run, query for the entities that already
			// exist so we can report what would be written if this was ran for
			// real.
			out := postgres.Where(&db.Result{Parent: r.Parent, Name: r.Name, ID: r.ID}).First(&db.Result{})
			if out.Error != nil {
				if errors.Is(out.Error, gorm.ErrRecordNotFound) {
					outcomes[key] = OutcomeDryRun
					continue
				}
				return outcomes, fmt.Errorf("error querying result: %w", out.Error)
			}
			outcomes[key] = OutcomeAlreadyExists
		}
	}

	var records []*db.Record
	out = mysql.Find(&records)
	if err := out.Error; err != nil {
		return outcomes, fmt.Errorf("error reading MySQL records: %w", err)
	}

	for _, r := range records {
		key := fmt.Sprintf("%s/results/%s/records/%s", r.Parent, r.ResultName, r.Name)

		if write {
			if err := convertRecord(r); err != nil {
				return outcomes, fmt.Errorf("error upgrading Record: %v", err)
			}
			out := postgres.Create(&r)
			if err := out.Error; err != nil {
				var perr *pgconn.PgError
				if errors.As(err, &perr) && perr.Code == UniqueViolationErrCode {
					outcomes[key] = OutcomeAlreadyExists
					continue
				} else {
					return outcomes, fmt.Errorf("error creating postgres record: %w", out.Error)
				}
			}
			outcomes[key] = OutcomeSuccess
		} else {
			out := postgres.Where(&db.Record{Parent: r.Parent, Name: r.Name, ID: r.ID}).First(&db.Record{})
			if out.Error != nil {
				if errors.Is(out.Error, gorm.ErrRecordNotFound) {
					outcomes[key] = OutcomeDryRun
					continue
				}
				return outcomes, fmt.Errorf("error querying record: %w", out.Error)
			}
			outcomes[key] = OutcomeAlreadyExists
		}
	}

	return outcomes, nil
}

func convertRecord(r *db.Record) error {
	any := new(anypb.Any)
	if err := proto.Unmarshal(r.Data, any); err != nil {
		return fmt.Errorf("error reading record data: %v", err)
	}

	// Convert Any proto to JSON data.
	out, err := anypb.UnmarshalNew(any, proto.UnmarshalOptions{})
	if err != nil {
		return fmt.Errorf("error unmarshalling mysql record data: %w", err)
	}

	// Handle type-specific conversions. This includes:
	//
	// - Convert proto TypeURL to our own custom format
	// - Handle proto int64 conversion (normally marshals to string, but we need
	//   a proper int - see https://github.com/protocolbuffers/protobuf/issues/8331
	//   for more details). We handle this by stripping out the problematic fields,
	//   converting the type, then resetting the fields later.
	var gen, observedGen int64
	var wantType any
	switch m := out.(type) {
	case *pb.TaskRun:
		r.Type = "tekton.dev/v1.TaskRun"
		wantType = &v1.TaskRun{}

		gen = m.GetMetadata().GetGeneration()
		m.Metadata.Generation = 0
		observedGen = m.GetStatus().GetObservedGeneration()
		m.Status.ObservedGeneration = 0
	case *pb.PipelineRun:
		r.Type = "tekton.dev/v1.PipelineRun"
		wantType = &v1.PipelineRun{}

		gen = m.GetMetadata().GetGeneration()
		m.Metadata.Generation = 0
		observedGen = m.GetStatus().GetObservedGeneration()
		m.Status.ObservedGeneration = 0
	default:
		r.Type = any.TypeUrl
		wantType = map[string]any{}
	}

	// Sanity check that the converted JSON can be safely marshalled into
	// expected well-known types.
	protob, err := protojson.Marshal(out)
	if err != nil {
		return fmt.Errorf("error converting proto to json: %v", err)
	}
	if err := json.Unmarshal(protob, wantType); err != nil {
		return fmt.Errorf("proto data does not match desired type: %w", err)
	}

	// Reset fields that were previously stripped out for compatibility.
	switch t := wantType.(type) {
	case *v1.TaskRun:
		t.Generation = gen
		t.Status.ObservedGeneration = observedGen
	case *v1.PipelineRun:
		t.Generation = gen
		t.Status.ObservedGeneration = observedGen
	}

	// Store original type back into the DB.
	b, err := json.Marshal(wantType)
	if err != nil {
		return fmt.Errorf("error marshalling output type: %w", err)
	}
	r.Data = b

	return nil
}
