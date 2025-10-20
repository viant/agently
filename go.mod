module github.com/viant/agently

go 1.25.1

require (
	github.com/aws/aws-sdk-go-v2 v1.32.2
	github.com/aws/aws-sdk-go-v2/config v1.28.0
	github.com/aws/aws-sdk-go-v2/service/bedrockruntime v1.8.1
	github.com/google/uuid v1.6.0
	github.com/jessevdk/go-flags v1.5.0
	github.com/stretchr/testify v1.10.0
	github.com/viant/afs v1.26.3
	github.com/viant/afsc v1.12.0
	github.com/viant/datly v0.20.17
	github.com/viant/embedius v0.2.5
	github.com/viant/fluxor v0.4.3
	github.com/viant/forge v0.2.16
	github.com/viant/jsonrpc v0.11.0
	github.com/viant/linager v0.0.0-20250503232524-71e07f0aeb99
	github.com/viant/mcp v0.7.0
	github.com/viant/mcp-protocol v0.5.10
	github.com/viant/scy v0.25.3
	github.com/viant/structology v0.7.1 // indirect
	github.com/viant/velty v0.2.1-0.20230927172116-ba56497b5c85
	github.com/viant/x v0.3.0 // indirect
	github.com/viant/xdatly/handler v0.0.0-20251006174948-cb34263ae8aa
	github.com/viant/xdatly/types/core v0.0.0-20250418144853-029d9a05ae20
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect -- replaced with internal stub, see below
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/aws/aws-sdk-go-v2/service/sts v1.32.2
	github.com/fsnotify/fsnotify v1.6.0
	github.com/golang-jwt/jwt/v5 v5.2.2
	github.com/minio/highwayhash v1.0.3
	github.com/openai/openai-go/v3 v3.0.0
	github.com/pmezard/go-difflib v1.0.0
	github.com/rwtodd/Go.Sed v0.0.0-20250326002959-ba712dc84b62
	github.com/sourcegraph/go-diff v0.7.0
	github.com/viant/gds v0.6.0
	github.com/viant/gosh v0.2.4
	github.com/viant/parsly v0.3.3
	github.com/viant/xdatly v0.5.4-0.20251006174948-cb34263ae8aa
	github.com/viant/xdatly/types/custom v0.0.0-20250418144853-029d9a05ae20
	golang.org/x/crypto v0.41.0
	golang.org/x/oauth2 v0.30.0
	modernc.org/sqlite v1.18.1
)

require (
	cel.dev/expr v0.16.2 // indirect
	cloud.google.com/go v0.116.0 // indirect
	cloud.google.com/go/auth v0.9.8 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.4 // indirect
	cloud.google.com/go/compute/metadata v0.6.0 // indirect
	cloud.google.com/go/firestore v1.17.0 // indirect
	cloud.google.com/go/iam v1.2.1 // indirect
	cloud.google.com/go/longrunning v0.6.1 // indirect
	cloud.google.com/go/monitoring v1.21.1 // indirect
	cloud.google.com/go/secretmanager v1.14.1 // indirect
	cloud.google.com/go/storage v1.45.0 // indirect
	firebase.google.com/go v3.13.0+incompatible // indirect
	firebase.google.com/go/v4 v4.14.0 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.24.3 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric v0.48.3 // indirect
	github.com/GoogleCloudPlatform/opentelemetry-operations-go/internal/resourcemapping v0.48.3 // indirect
	github.com/MicahParks/keyfunc v1.9.0 // indirect
	github.com/aerospike/aerospike-client-go v4.5.2+incompatible // indirect
	github.com/aerospike/aerospike-client-go/v6 v6.15.1 // indirect
	github.com/aws/aws-lambda-go v1.31.0 // indirect
	github.com/aws/aws-sdk-go v1.51.23 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.6 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.17.41 // indirect
	github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue v1.10.7 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.17 // indirect
	github.com/aws/aws-sdk-go-v2/feature/s3/manager v1.17.33 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.21 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.21 // indirect
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
	github.com/aws/smithy-go v1.22.0 // indirect
	github.com/census-instrumentation/opencensus-proto v0.4.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cncf/xds/go v0.0.0-20240905190251-b4127c9b8d78 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.2.0 // indirect
	github.com/envoyproxy/go-control-plane v0.13.1 // indirect
	github.com/envoyproxy/protoc-gen-validate v1.1.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/francoispqt/gojay v1.2.13 // indirect
	github.com/go-errors/errors v1.5.1 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-sql-driver/mysql v1.7.1 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/golang-jwt/jwt/v4 v4.5.1 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/s2a-go v0.1.8 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.4 // indirect
	github.com/googleapis/gax-go/v2 v2.13.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/lestrrat-go/backoff/v2 v2.0.8 // indirect
	github.com/lestrrat-go/blackmagic v1.0.2 // indirect
	github.com/lestrrat-go/httpcc v1.0.1 // indirect
	github.com/lestrrat-go/iter v1.0.2 // indirect
	github.com/lestrrat-go/jwx v1.2.29 // indirect
	github.com/lestrrat-go/option v1.0.1 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mazznoer/csscolorparser v0.1.3 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/nxadm/tail v1.4.11 // indirect
	github.com/onsi/ginkgo/v2 v2.23.3 // indirect
	github.com/onsi/gomega v1.37.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20200410134404-eec4a21b6bb0 // indirect
	github.com/richardlehane/mscfb v1.0.4 // indirect
	github.com/richardlehane/msoleps v1.0.3 // indirect
	github.com/tidwall/gjson v1.14.4 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/viant/aerospike v0.2.11-0.20241108195857-ed524b97800d // indirect
	github.com/viant/bigquery v0.5.1 // indirect
	github.com/viant/bindly v0.1.0 // indirect
	github.com/viant/bintly v0.2.0 // indirect
	github.com/viant/cloudless v1.12.0 // indirect
	github.com/viant/dyndb v0.1.4-0.20221214043424-27654ab6ed9c // indirect
	github.com/viant/firebase v0.1.1 // indirect
	github.com/viant/gmetric v0.3.2 // indirect
	github.com/viant/godiff v0.4.1 // indirect
	github.com/viant/govalidator v0.3.1 // indirect
	github.com/viant/igo v0.2.0 // indirect
	github.com/viant/pgo v0.11.0 // indirect
	github.com/viant/sqlparser v0.8.1 // indirect
	github.com/viant/sqlx v0.19.0 // indirect
	github.com/viant/structql v0.5.2 // indirect
	github.com/viant/tagly v0.2.2 // indirect
	github.com/viant/toolbox v0.37.1-0.20241210210120-32350b063cf8 // indirect
	github.com/viant/vec v0.2.4-0.20250304201558-76d42a0819d7 // indirect
	github.com/viant/xdatly/extension v0.0.0-20231013204918-ecf3c2edf259 // indirect
	github.com/viant/xlsy v0.3.1 // indirect
	github.com/viant/xmlify v0.1.1 // indirect
	github.com/viant/xreflect v0.7.3 // indirect
	github.com/viant/xunsafe v0.10.3 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xuri/efp v0.0.0-20230802181842-ad255f2331ca // indirect
	github.com/xuri/excelize/v2 v2.8.0 // indirect
	github.com/xuri/nfp v0.0.0-20230819163627-dc951e3ffe1a // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/detectors/gcp v1.31.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.56.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.56.0 // indirect
	go.opentelemetry.io/otel v1.36.0 // indirect
	go.opentelemetry.io/otel/metric v1.36.0 // indirect
	go.opentelemetry.io/otel/sdk v1.36.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.31.0 // indirect
	go.opentelemetry.io/otel/trace v1.36.0 // indirect
	golang.org/x/mod v0.26.0 // indirect
	golang.org/x/net v0.42.0 // indirect
	golang.org/x/sync v0.16.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	golang.org/x/time v0.7.0 // indirect
	golang.org/x/tools v0.35.0 // indirect
	google.golang.org/api v0.201.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/appengine/v2 v2.0.2 // indirect
	google.golang.org/genproto v0.0.0-20241015192408-796eee8c2d53 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20241015192408-796eee8c2d53 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241015192408-796eee8c2d53 // indirect
	google.golang.org/grpc v1.67.1 // indirect
	google.golang.org/grpc/stats/opentelemetry v0.0.0-20241017163036-56df169480cd // indirect
	google.golang.org/protobuf v1.36.5 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	lukechampine.com/uint128 v1.2.0 // indirect
	modernc.org/cc/v3 v3.36.3 // indirect
	modernc.org/ccgo/v3 v3.16.9 // indirect
	modernc.org/libc v1.17.1 // indirect
	modernc.org/mathutil v1.5.0 // indirect
	modernc.org/memory v1.2.1 // indirect
	modernc.org/opt v0.1.3 // indirect
	modernc.org/strutil v1.1.3 // indirect
	modernc.org/token v1.0.0 // indirect
)
