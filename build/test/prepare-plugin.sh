#/bin/bash
set -x

GIT_DEFAULT_BRANCH="main"
GIT_REPO="https://github.com/mattermost/mattermost-plugin-calls"
GIT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
PLUGIN_ID="com.mattermost.calls"

if git ls-remote --exit-code --heads ${GIT_REPO} ${GIT_BRANCH} ; then
  echo "Remote branch found"
else
  echo "Remote branch not found, using default"
  GIT_BRANCH=${GIT_DEFAULT_BRANCH}
fi

# Build
cd .. && git clone -b ${GIT_BRANCH} https://github.com/mattermost/mattermost-plugin-calls --depth 1 && \
cd mattermost-plugin-calls &&
cd standalone && npm ci && cd .. && \
cd webapp && npm ci && cd .. && \
echo 'replace github.com/mattermost/rtcd => ../rtcd' >> go.mod && \ # We need to make sure we compile the plugin with the rtcd changes.
go mod tidy && \
make dist MM_SERVICESETTINGS_ENABLEDEVELOPER=true

# Installation
PLUGIN_BUILD_PATH=$(realpath dist/*.tar.gz)
PLUGIN_FILE_NAME=$(basename ${PLUGIN_BUILD_PATH})
docker cp ${PLUGIN_BUILD_PATH} mmserver_server_1:/mattermost && \
docker cp config_patch.json mmserver_server_1:/mattermost && \
docker exec mmserver_server_1 bin/mmctl --local plugin delete ${PLUGIN_ID} && \
docker exec mmserver_server_1 bin/mmctl --local config patch config_patch.json  && \
docker exec mmserver_server_1 bin/mmctl --local plugin add ${PLUGIN_FILE_NAME} && \
docker exec mmserver_server_1 bin/mmctl --local plugin enable ${PLUGIN_ID}
