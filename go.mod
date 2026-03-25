module github.com/viant/agently

go 1.25.5

require (
	github.com/aws/aws-sdk-go-v2 v1.41.2 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.28.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/bedrockruntime v1.50.0 // indirect
	github.com/google/uuid v1.6.0
	github.com/jessevdk/go-flags v1.6.1
	github.com/stretchr/testify v1.11.1
	github.com/viant/afs v1.30.0
	github.com/viant/afsc v1.17.0
	github.com/viant/agently-core v0.1.1-0.20260325235104-18e5157af85b
	github.com/viant/datly v0.37.1-0.20260314193138-14bdea91fe4e
	github.com/viant/embedius v0.5.3 // indirect
	github.com/viant/forge v0.3.0 // indirect
	github.com/viant/jsonrpc v0.17.1-0.20260312173925-ef36fc39bce0 // indirect
	github.com/viant/linager v0.0.0-20250503232524-71e07f0aeb99 // indirect
	github.com/viant/mcp v0.14.0
	github.com/viant/mcp-protocol v0.11.0 // indirect
	github.com/viant/scy v0.25.3
	github.com/viant/structology v0.8.0 // indirect
	github.com/viant/velty v0.4.0 // indirect
	github.com/viant/x v0.4.1-0.20260306005005-975ded1e1bef // indirect
	github.com/viant/xdatly/handler v0.0.0-20251208172928-dd34b7f09fd5 // indirect
	github.com/viant/xdatly/types/core v0.0.0-20250418144853-029d9a05ae20 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect; indirect -- replaced with internal stub, see below
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/viant/bigquery v0.5.1
	golang.org/x/oauth2 v0.32.0
)

require (
	cel.dev/expr v0.24.0 // indirect
	cloud.google.com/go v0.116.0 // indirect
	cloud.google.com/go/auth v0.13.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.6 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	cloud.google.com/go/firestore v1.17.0 // indirect
	cloud.google.com/go/iam v1.2.2 // indirect
	cloud.google.com/go/longrunning v0.6.2 // indirect
	cloud.google.com/go/monitoring v1.21.2 // indirect
	cloud.google.com/go/secretmanager v1.14.2 // indirect
	cloud.google.com/go/storage v1.50.0 // indirect
	filippo.io/edwards25519 v1.1.0 // indirect
	firebase.google.com/go v3.13.0+incompatible // indirect
	firebase.google.com/go/v4 v4.14.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.30.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric v0.48.3 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping v0.48.3 // indirect
	github.com/MicahParks/keyfunc v1.9.0 // indirect
	github.com/aerospike/aerospike-client-go v4.5.2+incompatible // indirect
	github.com/aerospike/aerospike-client-go/v6 v6.15.1 // indirect
	github.com/aws/aws-lambda-go v1.31.0 // indirect
	github.com/aws/aws-sdk-go v1.51.23 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.5 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.17.41 // indirect
	github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue v1.10.7 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.17 // indirect
	github.com/aws/aws-sdk-go-v2/feature/s3/manager v1.17.33 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.18 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.18 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.1 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.3.21 // indirect
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.17.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/dynamodbstreams v1.13.27 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.4.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.7.20 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.18.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/s3 v1.66.0 // indirect
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.34.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/sns v1.31.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/sqs v1.34.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssm v1.55.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.24.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.28.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.32.2 // indirect
	github.com/aws/smithy-go v1.24.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cncf/xds/go v0.0.0-20251022180443-0feb69152e9f // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.2.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/envoyproxy/go-control-plane/envoy v1.35.0 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.2.1 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/francoispqt/gojay v1.2.13 // indirect
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/go-errors/errors v1.5.1 // indirect
	github.com/go-jose/go-jose/v4 v4.1.3 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-sql-driver/mysql v1.9.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.2.1 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.2 // indirect
	github.com/golang/groupcache v0.0.0-20241129210726-2c02b8208cf8 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/s2a-go v0.1.8 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.4 // indirect
	github.com/googleapis/gax-go/v2 v2.14.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/ledongthuc/pdf v0.0.0-20250511090121-5959a4027728 // indirect
	github.com/lestrrat-go/backoff/v2 v2.0.8 // indirect
	github.com/lestrrat-go/blackmagic v1.0.2 // indirect
	github.com/lestrrat-go/httpcc v1.0.1 // indirect
	github.com/lestrrat-go/iter v1.0.2 // indirect
	github.com/lestrrat-go/jwx v1.2.29 // indirect
	github.com/lestrrat-go/option v1.0.1 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mazznoer/csscolorparser v0.1.3 // indirect
	github.com/metakeule/fmtdate v1.1.2 // indirect
	github.com/minio/highwayhash v1.0.3 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/onsi/ginkgo/v2 v2.25.3 // indirect
	github.com/openai/openai-go/v3 v3.24.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/richardlehane/mscfb v1.0.4 // indirect
	github.com/richardlehane/msoleps v1.0.4 // indirect
	github.com/rwtodd/Go.Sed v0.0.0-20250326002959-ba712dc84b62 // indirect
	github.com/shakinm/xlsReader v0.9.12 // indirect
	github.com/sourcegraph/go-diff v0.7.0 // indirect
	github.com/spiffe/go-spiffe/v2 v2.6.0 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/tiendc/go-deepcopy v1.7.1 // indirect
	github.com/viant/aerospike v0.2.11-0.20241108195857-ed524b97800d // indirect
	github.com/viant/bintly v0.2.0 // indirect
	github.com/viant/cloudless v1.12.0 // indirect
	github.com/viant/dyndb v0.1.4-0.20221214043424-27654ab6ed9c // indirect
	github.com/viant/firebase v0.1.1 // indirect
	github.com/viant/gds v0.6.0 // indirect
	github.com/viant/gmetric v0.3.2 // indirect
	github.com/viant/godiff v0.4.1 // indirect
	github.com/viant/gosh v0.3.0 // indirect
	github.com/viant/govalidator v0.3.1 // indirect
	github.com/viant/igo v0.2.0 // indirect
	github.com/viant/parsly v0.3.3 // indirect
	github.com/viant/pgo v0.11.0 // indirect
	github.com/viant/sqlite-vec v0.3.1-0.20260220164201-136574510a3f // indirect
	github.com/viant/sqlparser v0.11.1-0.20260224194657-0470849e3588 // indirect
	github.com/viant/sqlx v0.22.0 // indirect
	github.com/viant/structql v0.5.4 // indirect
	github.com/viant/tagly v0.3.0 // indirect
	github.com/viant/toolbox v0.37.1-0.20241210210120-32350b063cf8 // indirect
	github.com/viant/vec v0.2.4-0.20250819200643-7e16b6ea443c // indirect
	github.com/viant/xdatly v0.5.4-0.20260306062123-17850ac34977 // indirect
	github.com/viant/xdatly/extension v0.0.0-20231013204918-ecf3c2edf259 // indirect
	github.com/viant/xdatly/types/custom v0.0.0-20250418144853-029d9a05ae20 // indirect
	github.com/viant/xlsy v0.3.1 // indirect
	github.com/viant/xmlify v0.1.1 // indirect
	github.com/viant/xreflect v0.7.5-0.20260314170600-13f09f37d46e // indirect
	github.com/viant/xunsafe v0.10.3 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xuri/efp v0.0.1 // indirect
	github.com/xuri/excelize/v2 v2.10.0 // indirect
	github.com/xuri/nfp v0.0.2-0.20250530014748-2ddeb826f9a9 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/detectors/gcp v1.38.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.56.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.56.0 // indirect
	go.opentelemetry.io/otel v1.38.0 // indirect
	go.opentelemetry.io/otel/metric v1.38.0 // indirect
	go.opentelemetry.io/otel/sdk v1.38.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.38.0 // indirect
	go.opentelemetry.io/otel/trace v1.38.0 // indirect
	golang.org/x/crypto v0.43.0 // indirect
	golang.org/x/exp v0.0.0-20251023183803-a4bb9ffd2546 // indirect
	golang.org/x/mod v0.29.0 // indirect
	golang.org/x/net v0.46.1-0.20251013234738-63d1a5100f82 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/sys v0.37.0 // indirect
	golang.org/x/text v0.30.0 // indirect
	golang.org/x/time v0.8.0 // indirect
	golang.org/x/tools v0.38.0 // indirect
	google.golang.org/api v0.214.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/appengine/v2 v2.0.2 // indirect
	google.golang.org/genproto v0.0.0-20241118233622-e639e219e697 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20251022142026-3a174f9686a8 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251022142026-3a174f9686a8 // indirect
	google.golang.org/grpc v1.77.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	modernc.org/libc v1.67.6 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
	modernc.org/sqlite v1.45.0 // indirect
)
