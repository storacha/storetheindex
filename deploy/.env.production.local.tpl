<%
PREFIX="${TF_WORKSPACE}-${TF_VAR_app}"

if [ "$TF_WORKSPACE" == "prod" ]; then
  BASE_TRACE_SAMPLE_RATIO="0.0001"
else
  BASE_TRACE_SAMPLE_RATIO="1.0"
fi

! IFS='' read -r -d '' Config <<EOC
{
  "Version": 2,
  "Identity": {
    "PeerID": "",
    "PrivKey": ""
  },
  "Addresses": {
    "Admin": "none",
    "Finder": "/ip4/0.0.0.0/tcp/3000",
    "Ingest": "none",
    "P2PAddr": "none",
    "NoResourceManager": true
  },
  "Bootstrap": {
    "Peers": [
      "/dns4/bootstrap-0.ipfsmain.cn/tcp/34721/p2p/12D3KooWQnwEGNqcM2nAcPtRR9rAX8Hrg4k9kJLCHoTR5chJfz6d",
      "/dns4/bootstrap-6.mainnet.filops.net/tcp/1347/p2p/12D3KooWP5MwCiqdMETF9ub1P3MbCvQCcfconnYHbWg6sUJcDRQQ",
      "/dns4/bootstrap-7.mainnet.filops.net/tcp/1347/p2p/12D3KooWRs3aY1p3juFjPy8gPN95PEQChm2QKGUCAdcDCC4EBMKf",
      "/dns4/bootstrap-8.mainnet.filops.net/tcp/1347/p2p/12D3KooWScFR7385LTyR4zU1bYdzSiiAb5rnNABfVahPvVSzyTkR",
      "/dns4/node.glif.io/tcp/1235/p2p/12D3KooWBF8cpp65hp2u9LK5mh19x67ftAam84z9LsfaquTDSBpt",
      "/dns4/bootstrap-2.mainnet.filops.net/tcp/1347/p2p/12D3KooWEWVwHGn2yR36gKLozmb4YjDJGerotAPGxmdWZx2nxMC4",
      "/dns4/bootstrap-1.starpool.in/tcp/12757/p2p/12D3KooWQZrGH1PxSNZPum99M1zNvjNFM33d1AAu5DcvdHptuU7u",
      "/dns4/bootstrap-0.mainnet.filops.net/tcp/1347/p2p/12D3KooWCVe8MmsEMes2FzgTpt9fXtmCY7wrq91GRiaC8PHSCCBj",
      "/dns4/bootstrap-1.mainnet.filops.net/tcp/1347/p2p/12D3KooWCwevHg1yLCvktf2nvLu7L9894mcrJR4MsBCcm4syShVc",
      "/dns4/bootstrap-0.starpool.in/tcp/12757/p2p/12D3KooWGHpBMeZbestVEWkfdnC9u7p6uFHXL1n7m1ZBqsEmiUzz",
      "/dns4/lotus-bootstrap.ipfsforce.com/tcp/41778/p2p/12D3KooWGhufNmZHF3sv48aQeS13ng5XVJZ9E6qy2Ms4VzqeUsHk",
      "/dns4/bootstrap-1.ipfsmain.cn/tcp/34723/p2p/12D3KooWMKxMkD5DMpSWsW7dBddKxKT7L2GgbNuckz9otxvkvByP"
    ],
    "MinimumPeers": 4
  },
  "Datastore": {
    "Type": "s3",
    "Dir": "${PREFIX}-datastore",
    "Region": "$TF_VAR_region",
    "TmpType": "none",
    "TmpDir": "",
    "TmpRegion": ""
  },
  "Discovery": {
    "FilterIPs": true,
    "IgnoreBadAdsTime": "2h0m0s",
    "Policy": {
      "Allow": true,
      "Except": null,
      "Publish": true,
      "PublishExcept": null
    },
    "PollInterval": "24h0m0s",
    "PollRetryAfter": "5h0m0s",
    "PollStopAfter": "168h0m0s",
    "PollOverrides": null,
    "ProviderReloadInterval": "10m0s",
    "UseAssigner": false
  },
  "Indexer": {
    "CacheSize": -1,
    "ConfigCheckInterval": "30s",
    "ShutdownTimeout": "15m",
    "ValueStoreType": "dynamodb",
    "DynamoDBRegion": "$TF_VAR_region",
    "DynamoDBProvidersTable": "${PREFIX}-valuestore-providers",
    "DynamoDBMultihashMapTable": "${PREFIX}-valuestore-multihash-map",
    "FreezeAtPercent": -1
  },
  "Ingest": {},
  "Logging": {
    "Level": "info",
      "Loggers": {
      "basichost": "warn",
      "bootstrap": "warn"
    }
  },
  "Peering": {
    "Peers": null
  },
  "Telemetry": {
    "ServiceName": "$PREFIX",
    "ExporterEndpoint": "https://api.honeycomb.io:443",
    "ExporterHeaders": "x-honeycomb-team=$HONEYCOMB_API_KEY",
    "SamplerRatio": $BASE_TRACE_SAMPLE_RATIO
  }
}
EOC

# GNU coreutils base64 (what comes installed in Ubuntu and most other linuxes) wraps
# lines at 76 characters by default. macOS base64 does not wrap by default and uses
# a different syntax to specify line wrapping (-b)
if [[ "$OSTYPE" == "linux-gnu"* ]]; then
  Config=$(echo "$Config" | base64 -w 0)
else
  Config=$(echo "$Config" | base64)
fi
%>

STORETHEINDEX_CONFIG=<%= $Config %>
