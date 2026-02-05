module github.com/wso2/ai-agent-management-platform/agent-manager-service

go 1.24.2

toolchain go1.24.9

require (
	github.com/golang-jwt/jwt/v5 v5.3.0
	github.com/jackc/pgx/v5 v5.7.1
	github.com/joho/godotenv v1.4.0
	github.com/stretchr/testify v1.11.1
	gorm.io/driver/postgres v1.6.0
	gorm.io/gorm v1.31.0
	k8s.io/apimachinery v0.34.1
)

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/net v0.47.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/utils v0.0.0-20250604170112-4c0f3b243397 // indirect
	sigs.k8s.io/json v0.0.0-20241014173422-cfa47c3a1cc8 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.0 // indirect
)

require (
	github.com/go-gormigrate/gormigrate/v2 v2.1.5
	github.com/google/uuid v1.6.0
	github.com/google/wire v0.7.0
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	go.uber.org/automaxprocs v1.6.0
	golang.org/x/crypto v0.45.0 // indirect
	golang.org/x/sync v0.18.0 // indirect
	golang.org/x/text v0.31.0 // indirect
)

require github.com/oapi-codegen/runtime v1.1.2

require github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/auth v0.0.0

replace github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/auth => ./clients/openchoreosvc/auth
