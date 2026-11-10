module github.com/mattermost/rtcd

go 1.26.10

require (
	git.mills.io/prologic/bitcask v1.0.2
	github.com/BurntSushi/toml v1.6.0
	github.com/gorilla/websocket v1.5.3
	github.com/grafana/pyroscope-go/godeltaprof v0.1.12
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/mattermost/logr/v2 v2.0.25
	github.com/mattermost/mattermost/server/public v0.6.1
	github.com/pborman/uuid v1.2.1
	github.com/pion/ice/v4 v4.2.0
	github.com/pion/interceptor v0.1.44
	github.com/pion/logging v0.2.4
	github.com/pion/rtcp v1.2.16
	github.com/pion/rtp v1.10.1
	github.com/pion/stun/v3 v3.1.1
	github.com/pion/webrtc/v4 v4.2.6
	github.com/prometheus/client_golang v1.25.0
	github.com/prometheus/procfs v0.23.0
	github.com/stretchr/testify v1.12.1
	github.com/vmihailenco/msgpack/v5 v5.4.1
	golang.org/x/crypto v0.58.0
	golang.org/x/sys v0.50.0
	golang.org/x/time v0.16.0
)

replace github.com/pion/interceptor v0.1.44 => github.com/bgardner8008/interceptor v0.1.44-mm-mods

require (
	github.com/Masterminds/semver/v3 v3.5.0 // indirect
	github.com/abcum/lcp v0.0.0-20201209214815-7a3f3840be81 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dyatlov/go-opengraph/opengraph v0.0.0-20220524092352-606d7b1e5f8a // indirect
	github.com/fatih/color v1.19.0 // indirect
	github.com/francoispqt/gojay v1.2.13 // indirect
	github.com/goccy/go-yaml v1.19.2 // indirect
	github.com/gofrs/flock v0.13.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gopherjs/gopherjs v1.17.2 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-plugin v1.8.0 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	github.com/klauspost/compress v1.20.4 // indirect
	github.com/mattermost/go-i18n v1.11.1-0.20211013152124-5c415071e404 // indirect
	github.com/mattermost/ldap v3.0.4+incompatible // indirect
	github.com/mattn/go-colorable v0.1.16 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/oklog/run v1.2.0 // indirect
	github.com/pandatix/go-abnf v0.5.5 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/philhofer/fwd v1.2.1 // indirect
	github.com/pion/datachannel v1.6.4 // indirect
	github.com/pion/dtls/v3 v3.1.15 // indirect
	github.com/pion/mdns/v2 v2.2.2 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/sctp v1.12.1 // indirect
	github.com/pion/sdp/v3 v3.0.20 // indirect
	github.com/pion/srtp/v3 v3.0.10 // indirect
	github.com/pion/transport/v4 v4.1.3 // indirect
	github.com/pion/transport/v5 v5.1.5 // indirect
	github.com/pion/turn/v4 v4.1.4 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/plar/go-adaptive-radix-tree v1.0.7 // indirect
	github.com/prometheus/client_model v0.6.3 // indirect
	github.com/prometheus/common v0.73.1 // indirect
	github.com/sirupsen/logrus v1.10.2 // indirect
	github.com/tinylib/msgp v1.6.5 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/wiggin77/merror v1.0.5 // indirect
	github.com/wiggin77/srslog v1.0.1 // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	go.yaml.in/yaml/v3 v3.0.6 // indirect
	golang.org/x/exp v0.0.0-20261109185158-a52e3aef5920 // indirect
	golang.org/x/mod v0.43.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261104224025-d1e00e9f33ab // indirect
	google.golang.org/grpc v1.85.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	gopkg.in/asn1-ber.v1 v1.0.0-20181015200546-f715ec2f112d // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

replace github.com/pion/ice/v4 => github.com/bgardner8008/ice/v4 v4.2.0-role-conflict-fix-v4
