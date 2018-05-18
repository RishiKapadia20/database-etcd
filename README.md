# Etcd Opspack

etcd is a distributed key/value store that provides a reliable way to store data across a cluster of machines. It’s open-source and available on GitHub. etcd gracefully handles leader elections during network partitions and will tolerate machine failure, including the leader

## What You Can Monitor

Monitors the performance and system status for an etcd database v2 and v3.

Note: Etcd version 3 is only supported when running etcd with API Version 2

## Service Checks

| Service Check | Description |
|:------------- | :------------- |
|Client Sent and Recieved Bytes|Sent and Received bytes for client (calculated using TimeSeries counter feature to work out the rate of change)|
|Durations|The latency durations of commits called by backend and fsync called by wal|
|File Descriptors|Max number and current number of file descriptors|
|HTTP Requests Made|Total number of HTTP requests made (calculated using TimeSeries counter feature to work out the rate of change)|
|Leader Changes Seen|The number of changes to the leader (calculated using TimeSeries counter feature to work out the rate of change)|
|Proposals|Number of proposals applied, committed, pending and failed to the system|
|Watchers|Number of current watchers|

## Setup and Configuration

To configure and utilize this Opspack, you simply need to add the 'Database - Etcd' Opspack to your Opsview Monitor system.

#### Step 1: Add the host template

![Add Host Template](/docs/img/add_etcd_host.png?raw=true)

#### Step 2: Add and configure variables required for this host

If the port used is not the default 2379 or you use an API version that is different from your etcd version then you will need to configure the ETCD variable

![Add Variables](/docs/img/add_etcd_variables.png?raw=true)

#### Step 3: Reload and the system will now be monitored

![View Host Service Checks](/docs/img/view_etcd_service_checks.png?raw=true)
