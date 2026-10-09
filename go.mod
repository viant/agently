module github.com/viant/agently

go 1.25.8

exclude google.golang.org/grpc/stats/opentelemetry v0.0.0-20240907200651-3ffb98b2c93a

require (
	github.com/aws/aws-sdk-go-v2 v1.41.9 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.32.20 // indirect
	github.com/aws/aws-sdk-go-v2/service/bedrockruntime v1.50.0 // indirect
	github.com/google/uuid v1.6.0
	github.com/jessevdk/go-flags v1.6.1
	github.com/stretchr/testify v1.11.1
	github.com/viant/afs v1.30.1-0.20260914155934-90e95d483861
	github.com/viant/afsc v1.18.0
	github.com/viant/agently-core v0.1.57-0.20261008235537-32026b7e271f
	github.com/viant/datly v1.1.1-0.20261008212701-79814253bce5
	github.com/viant/embedius v0.5.6 // indirect
	github.com/viant/forge v0.3.45-0.20261008230317-250262cef29a
	github.com/viant/jsonrpc v0.25.0
	github.com/viant/linager v0.0.0-20250503232524-71e07f0aeb99 // indirect
	github.com/viant/mcp v0.24.1-0.20261008165059-494f36529f6e
	github.com/viant/mcp-protocol v0.19.1-0.20261008202502-046707df5ed9
	github.com/viant/mcp-ui v0.2.0
	github.com/viant/scy v0.35.1-0.20260914041206-699e6c909726
	github.com/viant/structology v0.10.1-0.20261005184011-cdaab8ea7dab // indirect
	github.com/viant/velty v0.4.1-0.20260915052314-fca47c0595f8 // indirect
	github.com/viant/x v0.5.1-0.20260915043005-1bc42b9eef47 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect; indirect -- replaced with internal stub, see below
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/google/gops v0.3.28
	github.com/viant/agently-core/protocol/primitive v0.0.0-20261008224115-9803ba939e65
	github.com/viant/agently-core/protocol/resource v0.0.0-20261008223520-8ba096a165a0
	github.com/viant/authz v0.0.0-20261008202630-c983c9e64ee5
	github.com/viant/bigquery v0.5.4-0.20260927120042-a47888f8d2cc
)

require (
	cel.dev/expr v0.25.2 // indirect
	cloud.google.com/go v0.123.0 // indirect
	cloud.google.com/go/auth v0.20.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	cloud.google.com/go/firestore v1.22.0 // indirect
	cloud.google.com/go/iam v1.11.0 // indirect
	cloud.google.com/go/longrunning v1.0.0 // indirect
	cloud.google.com/go/monitoring v1.29.0 // indirect
	cloud.google.com/go/secretmanager v1.20.0 // indirect
	cloud.google.com/go/storage v1.62.2 // indirect
	codeberg.org/go-pdf/fpdf v0.10.0 // indirect
	filippo.io/edwards25519 v1.1.0 // indirect
	firebase.google.com/go/v4 v4.14.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.32.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric v0.56.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping v0.56.0 // indirect
	github.com/MicahParks/keyfunc v1.9.0 // indirect
	github.com/aerospike/aerospike-client-go v4.5.2+incompatible // indirect
	github.com/aws/aws-sdk-go v1.51.23 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.11 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.19.19 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.25 // indirect
	github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager v0.2.3 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.25 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.25 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.26 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.10 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.18 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.25 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.25 // indirect
	github.com/aws/aws-sdk-go-v2/service/s3 v1.102.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.41.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.1.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssm v1.68.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.30.19 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.36.2 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.42.3 // indirect
	github.com/aws/smithy-go v1.27.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cncf/xds/go v0.0.0-20260202195803-dba9d589def2 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.2.0 // indirect
	github.com/dlclark/regexp2/v2 v2.8.1 // indirect
	github.com/dop251/goja v0.0.0-20261007200356-e2ea74d3d210 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/envoyproxy/go-control-plane/envoy v1.37.0 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.3.3 // indirect
	github.com/evanphx/json-patch/v5 v5.9.11 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/francoispqt/gojay v1.2.13 // indirect
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/go-errors/errors v1.5.1 // indirect
	github.com/go-jose/go-jose/v4 v4.1.4 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-sourcemap/sourcemap v2.1.3+incompatible // indirect
	github.com/go-sql-driver/mysql v1.9.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.2.1 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/pprof v0.0.0-20250403155104-27863c87afa6 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.16 // indirect
	github.com/googleapis/gax-go/v2 v2.22.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.29.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/ledongthuc/pdf v0.0.0-20250511090121-5959a4027728 // indirect
	github.com/lestrrat-go/backoff/v2 v2.0.8 // indirect
	github.com/lestrrat-go/blackmagic v1.0.2 // indirect
	github.com/lestrrat-go/httpcc v1.0.1 // indirect
	github.com/lestrrat-go/iter v1.0.2 // indirect
	github.com/lestrrat-go/jwx v1.2.29 // indirect
	github.com/lestrrat-go/option v1.0.1 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-sqlite3 v1.14.16 // indirect
	github.com/mazznoer/csscolorparser v0.1.3 // indirect
	github.com/metakeule/fmtdate v1.1.2 // indirect
	github.com/minio/highwayhash v1.0.3 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/openai/openai-go/v3 v3.38.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/richardlehane/mscfb v1.0.4 // indirect
	github.com/richardlehane/msoleps v1.0.4 // indirect
	github.com/rwtodd/Go.Sed v0.0.0-20250326002959-ba712dc84b62 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	github.com/shakinm/xlsReader v0.9.12 // indirect
	github.com/sourcegraph/go-diff v0.7.0 // indirect
	github.com/spiffe/go-spiffe/v2 v2.6.0 // indirect
	github.com/tdewolff/parse/v2 v2.8.16 // indirect
	github.com/tidwall/gjson v1.19.0 // indirect
	github.com/tidwall/match v1.2.0 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/tiendc/go-deepcopy v1.7.1 // indirect
	github.com/viant/agently-core/protocol/window v0.0.0-20261008230757-163b77505d6a // indirect
	github.com/viant/agently-core/service/reportdefinition/materializer v0.0.0-20261008231612-4e2d839e8f05 // indirect
	github.com/viant/bindly v0.4.1-0.20261006234956-d0b4e58bac4e // indirect
	github.com/viant/bintly v0.2.0 // indirect
	github.com/viant/gds v0.6.0 // indirect
	github.com/viant/gmetric v0.3.2 // indirect
	github.com/viant/gosh v0.3.0 // indirect
	github.com/viant/govalidator v0.3.4-0.20260913213120-027a9dd54d73 // indirect
	github.com/viant/igo v0.2.0 // indirect
	github.com/viant/parsly v0.3.3 // indirect
	github.com/viant/sqlite-vec v0.3.1-0.20260220164201-136574510a3f // indirect
	github.com/viant/sqlparser v0.13.1-0.20261008165000-6fb4cdcd96a5 // indirect
	github.com/viant/sqlx v0.26.1-0.20261007205057-0330f64d8fd3 // indirect
	github.com/viant/structql v0.5.5-0.20261008164328-d0b837beaa3a // indirect
	github.com/viant/tagly v0.4.1-0.20261007173811-969600917520 // indirect
	github.com/viant/toolbox v0.39.0 // indirect
	github.com/viant/vec v0.2.4-0.20250819200643-7e16b6ea443c // indirect
	github.com/viant/xdatly v1.0.1-0.20261005144549-60dd64a21442 // indirect
	github.com/viant/xlsy v0.3.1 // indirect
	github.com/viant/xmlify v0.1.2-0.20260914155716-e525a8788fd0 // indirect
	github.com/viant/xreflect v0.7.5-0.20260314170600-13f09f37d46e // indirect
	github.com/viant/xunsafe v0.11.0 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xuri/efp v0.0.1 // indirect
	github.com/xuri/excelize/v2 v2.10.0 // indirect
	github.com/xuri/nfp v0.0.2-0.20250530014748-2ddeb826f9a9 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/detectors/gcp v1.44.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.69.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.69.0 // indirect
	go.opentelemetry.io/otel v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.44.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/sdk v1.44.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	go.opentelemetry.io/proto/otlp v1.10.0 // indirect
	golang.org/x/crypto v0.53.0 // indirect
	golang.org/x/exp v0.0.0-20251023183803-a4bb9ffd2546 // indirect
	golang.org/x/image v0.33.0 // indirect
	golang.org/x/mod v0.36.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.38.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	google.golang.org/api v0.283.0 // indirect
	google.golang.org/appengine/v2 v2.0.2 // indirect
	google.golang.org/genproto v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/grpc v1.81.1 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	modernc.org/libc v1.67.6 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
	modernc.org/sqlite v1.45.0 // indirect
)
