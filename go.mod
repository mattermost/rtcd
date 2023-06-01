module github.com/mattermost/rtcd

go 1.19

require (
	git.mills.io/prologic/bitcask v1.0.2
	github.com/BurntSushi/toml v1.0.0
	github.com/gorilla/websocket v1.5.0
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/mattermost/mattermost-server/server/public v0.0.4
	github.com/pborman/uuid v1.2.1
	github.com/pion/ice/v2 v2.3.2
	github.com/pion/interceptor v0.1.12
	github.com/pion/logging v0.2.2
	github.com/pion/rtcp v1.2.10
	github.com/pion/rtp v1.7.13
	github.com/pion/stun v0.4.0
	github.com/pion/webrtc/v3 v3.1.60
	github.com/prometheus/client_golang v1.15.0
	github.com/pyroscope-io/godeltaprof v0.1.1
	github.com/stretchr/testify v1.8.2
	github.com/vmihailenco/msgpack/v5 v5.3.5
	golang.org/x/crypto v0.8.0
	golang.org/x/sys v0.7.0
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0
)

replace github.com/pion/interceptor v0.1.12 => github.com/streamer45/interceptor v0.0.0-20230417184948-83c801a29470

require (
	github.com/abcum/lcp v0.0.0-20201209214815-7a3f3840be81 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/francoispqt/gojay v1.2.13 // indirect
	github.com/gofrs/flock v0.8.0 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/mattermost/logr/v2 v2.0.15 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/pion/datachannel v1.5.5 // indirect
	github.com/pion/dtls/v2 v2.2.6 // indirect
	github.com/pion/mdns v0.0.7 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/sctp v1.8.6 // indirect
	github.com/pion/sdp/v3 v3.0.6 // indirect
	github.com/pion/srtp/v2 v2.0.12 // indirect
	github.com/pion/transport/v2 v2.2.0 // indirect
	github.com/pion/turn/v2 v2.1.0 // indirect
	github.com/pion/udp/v2 v2.0.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/plar/go-adaptive-radix-tree v1.0.4 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.3.0 // indirect
	github.com/prometheus/common v0.42.0 // indirect
	github.com/prometheus/procfs v0.9.0 // indirect
	github.com/rogpeppe/go-internal v1.10.0 // indirect
	github.com/sirupsen/logrus v1.9.0 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/wiggin77/merror v1.0.4 // indirect
	github.com/wiggin77/srslog v1.0.1 // indirect
	golang.org/x/exp v0.0.0-20200908183739-ae8ad444f925 // indirect
	golang.org/x/net v0.9.0 // indirect
	google.golang.org/protobuf v1.30.0 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
