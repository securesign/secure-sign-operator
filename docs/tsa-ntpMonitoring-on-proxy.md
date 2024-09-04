# Configuring NTP Monitoring on a Proxy-Enabled OpenShift Cluster

In a proxy-enabled OpenShift cluster, configuring Network Time Protocol (NTP) monitoring for the timestamp authority requires a different approach than the standard configuration. This is primarily because NTP uses the UDP protocol, and the OpenShift proxy is not designed to handle UDP traffic, which typically results in failed NTP communication through the proxy.

## Prerequisites
Before you begin, ensure that:

1. You have a proxy enabled openshift cluster.
2. You have the necessary access to your openshift cluster.
3. You have the `chrony` command line utility installed.

There are two main options available for handling this:

1. Disabling NTP Monitoring:
NTP monitoring can be disabled by setting the following in the timestamp authority resource definition:

    ```
    ntpMonitoring:
        enabled: false
    ```

2. Configuring NTP monitoring to use internal NTP servers
    1. Go to Compute -> Nodes -> Open a terminal in any of the worker or master nodes.
    2. Enter the command `chroot /host` to be able to use the hosts binary's.
    3. Enter the command `chronyc sources` this will give you a list of the available ntp servers.
    4. You should see one source listed, an IP address.
    5. Update/Create the ntpMonitoring config for the Timestamp Authority Service:
        ```
            ntpMonitoring:
                enabled: true
                config:
                requestAttempts: 3
                requestTimeout: 5
                numServers: 1
                maxTimeDelta: 6
                serverThreshold: 1
                period: 60
                servers: 
                    - <chrony_source>
        ```
    6. The Timestamp Authority service should deploy successfully without error.
