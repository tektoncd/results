module github.com/tektoncd/results

go 1.13

require (
	github.com/Azure/go-autorest/autorest/validation v0.2.0 // indirect
	github.com/golang/protobuf v1.5.2
	github.com/google/cel-go v0.5.1
	github.com/google/go-cmp v0.5.7
	github.com/google/uuid v1.3.0
	github.com/gorilla/mux v1.8.0
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/jackc/pgconn v1.8.1
	github.com/jonboulle/clockwork v0.2.2
	github.com/lib/pq v1.10.2 // indirect
	github.com/mattn/go-sqlite3 v2.0.3+incompatible
	github.com/prometheus/client_golang v1.11.0
	github.com/tektoncd/pipeline v0.29.0
	go.uber.org/automaxprocs v1.4.0
	go.uber.org/zap v1.19.1
	golang.org/x/oauth2 v0.0.0-20211028175245-ba495a64dcb5
	google.golang.org/api v0.60.0
	google.golang.org/genproto v0.0.0-20211021150943-2b146023228c
	google.golang.org/grpc v1.42.0
	google.golang.org/protobuf v1.27.1
	gorm.io/driver/mysql v1.0.3
	gorm.io/driver/postgres v1.1.0
	gorm.io/driver/sqlite v1.1.4
	gorm.io/gorm v1.21.9
	k8s.io/api v0.21.4
	k8s.io/apimachinery v0.21.4
	k8s.io/client-go v0.21.4
	knative.dev/pkg v0.0.0-20211115071955-517ef0292b53
	sigs.k8s.io/yaml v1.3.0
)
