module github.com/mattermost/rtcd

go 1.23.6

require (
	git.mills.io/prologic/bitcask v1.0.2
	github.com/BurntSushi/toml v1.0.0
	github.com/gorilla/websocket v1.5.3
	github.com/grafana/pyroscope-go/godeltaprof v0.1.8
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/mattermost/mattermost/server/public v0.1.10
	github.com/pborman/uuid v1.2.1
	github.com/pion/ice/v4 v4.0.10
	github.com/pion/interceptor v0.1.40
	github.com/pion/logging v0.2.4
	github.com/pion/rtcp v1.2.15
	github.com/pion/rtp v1.8.21
	github.com/pion/stun/v3 v3.0.0
	github.com/pion/webrtc/v4 v4.1.3
	github.com/prometheus/client_golang v1.15.0
	github.com/prometheus/procfs v0.9.0
	github.com/stretchr/testify v1.10.0
	github.com/vmihailenco/msgpack/v5 v5.4.1
	golang.org/x/crypto v0.39.0
	golang.org/x/sys v0.33.0
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0
)

replace github.com/pion/interceptor v0.1.40 => github.com/streamer45/interceptor v0.0.0-20250701195358-9e4ca8111c7a

require (
	github.com/abcum/lcp v0.0.0-20201209214815-7a3f3840be81 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dyatlov/go-opengraph/opengraph v0.0.0-20220524092352-606d7b1e5f8a // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/francoispqt/gojay v1.2.13 // indirect
	github.com/go-asn1-ber/asn1-ber v1.5.7 // indirect
	github.com/gofrs/flock v0.8.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-plugin v1.6.3 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	github.com/klauspost/compress v1.17.10 // indirect
	github.com/mattermost/go-i18n v1.11.1-0.20211013152124-5c415071e404 // indirect
	github.com/mattermost/ldap v0.0.0-20231116144001-0f480c025956 // indirect
	github.com/mattermost/logr/v2 v2.0.21 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/oklog/run v1.1.0 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/philhofer/fwd v1.1.3-0.20240916144458-20a13a1f6b7c // indirect
	github.com/pion/datachannel v1.5.10 // indirect
	github.com/pion/dtls/v3 v3.0.6 // indirect
	github.com/pion/mdns/v2 v2.0.7 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/sctp v1.8.39 // indirect
	github.com/pion/sdp/v3 v3.0.14 // indirect
	github.com/pion/srtp/v3 v3.0.6 // indirect
	github.com/pion/transport/v3 v3.0.7 // indirect
	github.com/pion/turn/v4 v4.0.2 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/plar/go-adaptive-radix-tree v1.0.4 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.3.0 // indirect
	github.com/prometheus/common v0.42.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/tinylib/msgp v1.2.5 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/wiggin77/merror v1.0.5 // indirect
	github.com/wiggin77/srslog v1.0.1 // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	golang.org/x/exp v0.0.0-20200908183739-ae8ad444f925 // indirect
	golang.org/x/net v0.41.0 // indirect
	golang.org/x/text v0.26.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250124145028-65684f501c47 // indirect
	google.golang.org/grpc v1.70.0 // indirect
	google.golang.org/protobuf v1.36.4 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
