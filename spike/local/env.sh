#!/usr/bin/env bash
# Environment for running dwh-spike against the local docker stand-in.
# Source this file: `source spike/local/env.sh`
export DWH_ARCHIVE_S3_ENDPOINT="http://localhost:9000"
export DWH_ARCHIVE_BUCKET="dwh-observations"
export DWH_ARCHIVE_ACCESS_KEY_ID="dwhspike"
export DWH_ARCHIVE_SECRET_ACCESS_KEY="dwhspike-secret"
export DWH_ARCHIVE_PATH_STYLE="true"
export DWH_ARCHIVE_REGION="us-east-1"
export DWH_CONTROL_S3_ENDPOINT="http://localhost:9000"
export DWH_CONTROL_BUCKET="dwh-control"
export DWH_CONTROL_ACCESS_KEY_ID="dwhspike"
export DWH_CONTROL_SECRET_ACCESS_KEY="dwhspike-secret"
export DWH_CONTROL_PATH_STYLE="true"
export DWH_CONTROL_REGION="us-east-1"
export DWH_CATALOG_URI="http://localhost:8181"
export DWH_CATALOG_WAREHOUSE="s3://dwh-canonical/"
export DWH_CATALOG_TOKEN=""
export DWH_CATALOG_NAMESPACE="dwh_spike"
export DWH_CATALOG_PROPS="s3.endpoint=http://localhost:9000,s3.region=us-east-1,s3.access-key-id=dwhspike,s3.secret-access-key=dwhspike-secret,s3.force-virtual-addressing=false"
unset DWH_R2SQL_ACCOUNT_ID DWH_R2SQL_BUCKET DWH_R2SQL_TOKEN
