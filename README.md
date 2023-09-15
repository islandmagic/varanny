# varanny

`varanny` is designed to enhance the functionality of VARA, a popular software modem used in radio amateur digital transmissions. VARA operates via two TCP ports, one for commands and the other for data. The use of TCP/IP for interaction allows the VARA modem software to run on a headless computer connected to a radio, while client applications can be executed on a different network device. However, VARA wasn't designed to run as a service, hence it has certain limitations.

`varanny` steps in to address these limitations, acting as a 'nanny' for VARA. It offers two primary capabilities:

**Service Announcement**: `varanny` announces the service through DNS Service Discovery, enabling clients to discover an active VARA instance and automatically retrieve the IP and port configured for that instance.

The service is broadcasted as `_vara-modem._tcp` and contains a TXT entry for the ports of VARA FM and VARA HF applications represetned as `fm=8300,hf=8400` for example.

To test if the service is running, you can validate from a terminal on macOS

```
$ dns-sd -B _vara-modem._tcp
Browsing for _vara-modem._tcp
DATE: ---Sun 20 Aug 2023---
21:32:11.683  ...STARTING...
Timestamp     A/R    Flags  if Domain               Service Type         Instance Name
21:32:11.684  Add        2   9 local.               _vara-modem._tcp.    VARA Modem
```

and resolve the service address with

```
$ dns-sd -L "VARA Modem" _vara-modem._tcp local
Lookup VARA Modem._vara-modem._tcp.local
DATE: ---Sun 20 Aug 2023---
21:32:15.552  ...STARTING...
21:32:15.553  VARA\032Modem._vara-modem._tcp.local. can be reached at t4windoz.local.:8210 (interface 9)
 fm=8300 hf=8400
```
The servie accnouncment has been inspired by https://github.com/hessu/aprs-specs/blob/master/TCP-KISS-DNS-SD.md

**Remote Management**: `varanny` allows client applications to remotely start and stop the VARA program. This is particularly useful in headless applications, especially when VARA FM and VARA HF share the same sound card interface. Furthermore, VARA, when running on a *nix system via Wine, fails to rebind to its ports after a connection is closed. This means that the VARA application must be restarted after each connection, and `varanny` facilitates this process. This particular issue has been discussed in this [thread](https://groups.io/g/VARA-MODEM/topic/lunchbag_portable_hf_mail/97360073).

Supported commands

* `START VARAFM` - Starts the executable configured for VARA FM
* `STOP VARAFM` - Stops the VARA FM executable
* `START VARAHF` - Starts the executable configured for VARA HF
* `STOP VARAHF` - Stops the VARA HF executable
* `VERSION` - Returns version

## Installation

To set up `varanny`:

1. [Download](https://github.com/islandmagic/varanny/releases/latest) the zip file suitable for your platform.
1. Extract the zip into a local directory.
1. Launch the `varanny` executable. You could either place a shortcut in the startup folder or write a script to start it at boot time.

## Configuration

To configure `varanny`, edit the `varanny.json` file as per your needs. If you do not want the VARA executable to be managed by `varanny`, leave the executable path field empty ("").

### Running with Wine on Linux

Ensure VARA is installed in its default location and wine executable is in the PATH.

```
{
  "Name": "VARA Modem",
  "Port": 8273,
  "VaraFM" : {
    "Cmd": "wine",
    "Args": "C:\\VARA FM\\VARAFM.exe",
    "Port": 8300,
    "CatCtrl": {
      "Port": 4532,
      "Dialect": "hamlib"
    }    
  },
  "VaraHF" : {
    "Cmd": "wine",
    "Args": "C:\\VARA\\VARA.exe",
    "Port": 8400,
    "CatCtrl": {
      "Port": 4532,
      "Dialect": "hamlib"
    }    
  }
}
```
