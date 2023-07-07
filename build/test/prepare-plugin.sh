#/bin/bash
set -x

git clone https://github.com/mattermost/mattermost-plugin-calls --depth 1
cd mattermost-plugin-calls/standalone && npm ci && cd -
cd mattermost-plugin-calls/webapp && npm ci && cd -
cd mattermost-plugin-calls
echo "replace github.com/mattermost/rtcd => ../" >> go.mod
go mod tidy
make dist MM_SERVICESETTINGS_ENABLEDEVELOPER=true && cd -
PLUGIN_BUILD_PATH=$(realpath mattermost-plugin-calls/dist/*.tar.gz)
PLUGIN_FILE_NAME=$(basename ${PLUGIN_BUILD_PATH})
docker cp ${PLUGIN_BUILD_PATH} mmserver_server_1:/mattermost
docker exec mmserver_server_1 bin/mmctl user create --email sysadmin@sample.mattermost.com --username sysadmin --password Sys@dmin-sample1 --system-admin --local
docker exec mmserver_server_1 bin/mmctl --local plugin delete com.mattermost.calls
docker exec mmserver_server_1 bin/mmctl --local plugin add ${PLUGIN_FILE_NAME}
docker exec mmserver_server_1 bin/mmctl --local plugin enable com.mattermost.calls
