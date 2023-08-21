# varanny

`varanny` is designed to enhance the functionality of VARA, a popular software modem used in radio amateur digital transmissions. VARA operates via two TCP ports, one for commands and the other for data. The use of TCP/IP for interaction allows the VARA modem software to run on a headless computer connected to a radio, while client applications can be executed on a different network device. However, VARA wasn't designed to run as a service, hence it has certain limitations.

`varanny` steps in to address these limitations, acting as a 'nanny' for VARA. It offers two primary capabilities:

1. **Service Announcement**: `varanny` announces the service through dns-sd, enabling clients to discover an active VARA instance and automatically retrieve the IP and port configured for that instance.

1. **Remote Management**: `varanny` allows client applications to remotely start and stop the VARA program. This is particularly useful in headless applications, especially when VARA FM and VARA HF share the same sound card interface. Furthermore, VARA, when running on a *nix system via Wine, fails to rebind to its ports after a connection is closed. This means that the VARA application must be restarted after each connection, and `varanny` facilitates this process.

## Installation

To set up `varanny`:

1. Download the zip file suitable for your platform.
1. Extract the zip into a local directory.
1. Launch the `varanny` executable. You could either place a shortcut in the startup folder or write a script to start it at boot time.

## Configuration

To configure `varanny`, edit the `varanny.json` file as per your needs. If you do not want the VARA executable to be managed by `varanny`, leave the executable path field empty ("").