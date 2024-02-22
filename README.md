# varanny

`varanny` is an enhancement tool for VARA, a widely used software modem in amateur radio digital transmissions. VARA functions through two TCP ports - one for commands, the other for data. This TCP/IP interaction enables the VARA modem software to run on a radio-connected, headless computer, while client applications operate on a separate network device. However, since VARA wasn't designed to function as a service, this setup comes with certain limitations.

## Overview

```
┌────────────────────┐                ┌──────────────────────────┐              ┌────────────────────┐
│ ┌────────────────┐ │                │      ┌──────────────┐    │              │                    │
│ │                │ │                │      │              │    │              │                    │
│ │                │ │                │      │ ctrl         │    │              │                    │
│ │                ├─┼────port 8273───┼──────▶              │    │              │       Radio        │
│ │                │ │                │      │   varanny    │    │              │                    │
│ │                │ │        ┌───────┼──────┤              │    │              │                    │
│ │                │ │        │       │      │              │    │              │                    │
│ │                │ │        │       │      │    manage    │    │              └──────┬─┬─┬─┬───────┘
│ │                │ │        │       │      └──┬─────────┬─┘    │                     │ │ │ │        
│ │                │ │        │       │         │         │      │                     │ │ │ │ TX     
│ │                │ │     DNS-SD     │         │         │      │            Headset  │ │ │ │ RX     
│ │                │ │     Service    │         │         ▽      │             Jack    │ │ │ │ PTT    
│ │                │ │  Announcement  │         │  ┌ ─ ─ ─ ─ ─   │                     │ │ │ │ GND    
│ │                │ │        │       │         │    VARA.ini │┐ │                     │ │ │ │        
│ │   RadioMail    │ │        │       │         │  └ ─ ─ ─ ─ ─   │                     │ │ │ │        
│ │                │ │        │       │         │    └ ─ ─ ─ ─ ┘ │              ┌──────┴─┴─┴─┴───────┐
│ │                │ │        ▽       │         ▽                │              │                    │
│ │                │ │                │      ┌─────────────┐     │              │   ┌────────────┐   │
│ │                ◀─┼──CMD port 8300─┼──────▶             ├─────┼──Audio out───┼───▶            │   │
│ │                │ │                │      │    VARA     │     │              │   │ Soundcard  │   │
│ │                ◀─┼─DATA port 8301─┼──────▶             ◀─────┼───Audio in───┼───┤            │   │
│ │                │ │                │      └─────────────┘     │              │   └────────────┘   │
│ │                │ │                │                          │              │                    │
│ │                │ │                │      ┌─────────────┐     │              │   ┌────────────┐   │
│ │                │ │                │      │             │     │              │   │            │   │
│ │                ◀─┼───port 4532────┼──────▶   rigctld   │─────┼────usb───────┼───▶    PTT     │   │
│ │                │ │                │      │             │     │              │   │            │   │
│ │                │ │                │      └─────────────┘     │              │   └────────────┘   │
│ └────────────────┘ │                │                          │              │                    │
│                    │      WiFi      │                          │     USB      │                    │
│    iPhone (iOS)    │                │    Headless Computer     │              │ Sound Card Adapter │
└────────────────────┘                └──────────────────────────┘              └────────────────────┘
```

`varanny` steps in to address these limitations, acting as a 'nanny' for VARA. It offers two primary capabilities:

## Service Announcement
`varanny` announces the service through DNS Service Discovery, enabling clients to discover an active VARA instance and automatically retrieve the IP and command port configured for that instance. In VARA modems, the data port number is always one more than the command port number.

The service are broadcasted as `_vara-modem._tcp` and contain a TXT entry with `;` separated options.

### Supported TXT options
* `type=` type of modem, `fm` or `hf`.
* `launchport=` port of varanny launcher.
* `catport=` port of the cat control daemon, if any.
* `catdialect=` type of cat control daemon. Currently only `hamlib` is supported.

To test if the service is running, you can validate from a terminal on macOS

```
$ dns-sd -B _vara-modem._tcp
Browsing for _vara-modem._tcp
DATE: ---Tue 24 Oct 2023---
18:20:18.020  ...STARTING...
Timestamp     A/R    Flags  if Domain               Service Type       Instance Name
18:20:18.021  Add        3   1 local.               _vara-modem._tcp.  VARA HF Modem
18:20:18.021  Add        3   6 local.               _vara-modem._tcp.  VARA HF Modem
18:20:18.021  Add        2   7 local.               _vara-modem._tcp.  VARA HF Modem
```

and resolve the service address with

```
$ dns-sd -L "VARA HF Modem" _vara-modem._tcp local
Lookup VARA HF Modem._vara-modem._tcp.local
DATE: ---Tue 24 Oct 2023---
18:21:15.325  ...STARTING...
18:21:15.326  VARA\032HF\032Modem._vara-modem._tcp.local. can be reached at cervin.local.local.:8400 (interface 1) Flags: 1
 type=hf\; launchport=8273\; catport=4532\; catdialect=hamlib\;
```

The service announcement has been inspired by https://github.com/hessu/aprs-specs/blob/master/TCP-KISS-DNS-SD.md

## Remote Management
`varanny` allows client applications to remotely start and stop the VARA program. This is particularly useful in headless applications, especially when VARA FM and VARA HF share the same sound card interface. Furthermore, VARA, when running on a *nix system via Wine, fails to rebind to its ports after a connection is closed. This means that the VARA application must be restarted after each connection, and `varanny` facilitates this process. This particular issue has been discussed in this [thread](https://groups.io/g/VARA-MODEM/topic/lunchbag_portable_hf_mail/97360073).

### Supported commands
Connections to `varanny` are session-oriented. A client connects, requests to start a modem, performs some operations, and then stops it. Once the modem is stopped, `varanny` will close the connection and restore the VARA configuration file if necessary.

* `list` - List the available modem names
* `start <modem name>` - Starts the modem and rig control defined for `<modem name>`
* `stop` - Stops the processes and close the connection
* `monitor <modem name>` - Connects to the input audio interface defined for this modem. Returns the interface name, followed by continous stream of audio level in dbFS.
* `config` - Echo the `varanny.json` config file content
* `version` - Returns varanny version

### Multiple Configurations
VARA doesn't offer command line configuration options. Therefore, changes like sound card name, PTT com port, etc., need to be made through its GUI. `varanny` can help manage multiple configurations for you. It automatically swaps the `.ini` configuration file that VARA reads, allowing for seamless configuration changes before each session and restoring the default settings afterward. To create a new configuration, follow these steps:  

1. Open the VARA application. 
1. Adjust the parameters to your preference. 
1. Close the application. 
1. Go to the VARA installation directory (`C:\VARA` or `C:\VARA FM` by default) and duplicate the `VARA.ini` or `VARAFM.ini` file. 

It's a good idea to use a naming convention that reflects what the configuration file represents. For instance, you might name a configuration set up for a Digirig sound card `VARA.digirig.ini`. You can then specify the configuration to use in the `varanny` config file. Also make a backup copy of your `.ini` in case something goes wrong and you need to restore it manually.

## Installation
To set up `varanny`:

1. [Download](https://github.com/islandmagic/varanny/releases/latest) the zip file suitable for your platform.
1. Extract the zip into a local directory. Note: Windows makes you jump thru all sorts of hoops to run an `.exe` you downloaded from the internet. You should be used to it by now.
1. Launch the `varanny` executable. You could either place a shortcut in the startup folder or write a script to start it at boot time.


### Building from Source
Alternatively, you can build `varanny` from source.
Install go for your platform https://go.dev
```
git checkout git@github.com:islandmagic/varanny.git
cd varanny
CGO_ENABLED=1 go build
```

When building on Raspberry Pi arm, you may need:
```
sudo apt-get install -y libatomic1
CGO_ENABLED=1 CGO_LDFLAGS="-latomic" go build
```

To cross compile a Windows build from a *nix OS:
```
CC=x86_64-w64-mingw32-gcc GOOS=windows GOARCH=amd64 CGO_ENABLED=1 go build
```

## Configuration
To configure `varanny`, edit the `varanny.json` file as per your needs.

### Configuration Attributes
Configuration must be a valid `.json` file. You can define as many "modems" as you'd like. This can be handy if you use the same computer to connect to multiple radios that require different configurations. If you need to run VARA with different parameters, simply clone your VARA `.ini` configuration file and specify it as the `Config` attribute in `varanny.json`.

* `Port` port that `varanny` agent binds to. Default is 8273.
* `Delay` delay before `varanny` binds to a network interface. This is useful to let some time for other software to establish a HotSpot configuration when booting up. Default is set to 10s.
* `AudioInputNameThreshold` an optional value between 0 (completely different) and 1 (exact match). Specifies how different the name of the audio input interface can be between what's in `VARA.ini` and the system to be considered a match. Default is 0.7.
* `Modems` arrray containing modem definitions.
   * `Name` name the modem will be advertised under. **Must be unique**.
   * `Type` type of VARA modem, `fm` or `hf`.
   * `Cmd` fully qualified path to the executable to start this VARA modem. Note for Windows paths, the backslash separators must be escaped using `\\`
   * `Args` optional arguments to pass to the executable.
   * `AudioInputName` an optional value to specify the system audio input interface name. If present, `varanny` will use this over what is specified in `VARA.ini`
   * `Config` optional path to a VARA configuration file. If present, upon starting a session, a backup of the existing `VARA.ini` or `VARAFM.ini` file is created and then the specified configuration file is applied. Once the session concludes, the original `.ini` file is restored. This feature ensures the preservation of original settings while enabling different configurations for specific setups such as a sound card name.
   * `CatCtrl` optional CAT control definition.
      * `Port` port used by the CAT control agent.
      * `Dialect` protocol used by the CAT control agent. Currently only `hamlib` is supported.
      * `Cmd` fully qualified path to the executable to start the CAT control agent. Note for Windows paths, the backslash separators must be escaped using `\\`
      * `Args` optional arguments to pass to the executable.

[Sample Configuration](https://github.com/islandmagic/varanny/blob/master/varanny.json)

### Running VARA with Wine on Linux
Ensure VARA is installed in its default location and wine executable is in the PATH. Here is an sample configuration that defines two profiles for FM connections and one for HF.

```
{
  "Port": 8273,
  "Modems": [
    {
      "Name": "IC705FM",
      "Type": "fm",
      "Cmd": "wine",
      "Args": "/home/georges/.wine/drive_c/VARA FM/VARAFM.exe",
      "Config": "/home/georges/.wine/drive_c/VARA FM/VARAFM.ic705.ini",
      "CatCtrl": {
        "Port": 4532,
        "Dialect": "hamlib",
        "Cmd": "rigctld",
        "Args": "-m 3085 -r /dev/ic-705a -s 19200"
      }
    },
    {
      "Name": "THD74",
      "Type": "fm",
      "Cmd": "wine",
      "Args": "/home/georges/.wine/drive_c/VARA FM/VARAFM.exe",
      "Config": "/home/georges/.wine/drive_c/VARA FM/VARAFM.thd74.ini",
      "CatCtrl": {
        "Port": 4532,
        "Dialect": "hamlib",
        "Cmd": "rigctld",
        "Args": "-p /dev/ttyUSB0 -P RTS"
      }
    },    
    {
      "Name": "IC705HF",
      "Type": "hf",
      "Cmd": "wine",
      "Args": "/home/georges/.wine/drive_c/VARA/VARA.exe",
      "CatCtrl": {
        "Port": 4532,
        "Dialect": "hamlib",
        "Cmd": "rigctld",
        "Args": "-m 3085 -r /dev/ic-705a -s 19200"
      }
    }
  ]
}
```

## RadioMail Integration 

[RadioMail](https://radiomail.app), the winlink email app for iOS has integrated native support for `varanny`. See it in action:

https://github.com/islandmagic/varanny/assets/3819/08a114bf-8e54-412f-bad4-21064aff69c1

### VARA HotSpot Setup Guides

Follow these guides and watch the videos to learn how to utilize `varanny` to create a VARA hotspot on a headless computer and send and receive emails from anywhere.

* [VARA HotSpot Windows Setup Guide](https://github.com/islandmagic/varanny/wiki/VARA-HotSpot-Windows-Setup-Guide)
* [VARA HotSpot Linux Setup Guide](https://github.com/islandmagic/varanny/wiki/VARA-HotSpot-Linux-Setup-Guide)

[![Video](https://img.youtube.com/vi/7bOmWUk1XhM/maxresdefault.jpg)](https://www.youtube.com/watch?v=7bOmWUk1XhM)

[![Video](https://img.youtube.com/vi/kPDTH4L319Q/maxresdefault.jpg)](https://www.youtube.com/watch?v=kPDTH4L319Q)

## Troubleshooting

To validate your configuration, the easiest is to run an interactive session against `varanny` from another computer on the same network.

1. Start `varanny` on the host.
2. From a different computer, verify that modems are being advertised on the network. The following commands should list the various modems defined in you `.json` configuration file.

On macOS

```
$ dns-sd -B _vara-modem._tcp
Browsing for _vara-modem._tcp
DATE: ---Mon 06 Nov 2023---
16:15:31.372  ...STARTING...
Timestamp     A/R    Flags  if Domain               Service Type         Instance Name
16:15:31.373  Add        3   7 local.               _vara-modem._tcp.  THD74
16:15:31.373  Add        3   7 local.               _vara-modem._tcp.  IC705FM
```

On Linux

```
$ avahi-browse -v _vara-modem._tcp
Server version: avahi 0.8; Host name: t4.local
E Ifce Prot Name                                          Type                 Domain
+ wlp2s0 IPv6 IC705FM                                       _vara-modem._tcp   local
+ wlp2s0 IPv6 THD74FM                                       _vara-modem._tcp   local
+ wlp2s0 IPv4 THD74FM                                       _vara-modem._tcp   local
+ wlp2s0 IPv4 IC705FM                                       _vara-modem._tcp   local
: Cache exhausted
: All for now
```

3. Lookup the host address for a particular modem.

On macOS

```
$ dns-sd -L "IC705FM" _vara-modem._tcp local
Lookup IC705FM._vara-modem._tcp.local
DATE: ---Mon 06 Nov 2023---
 16:58:59.059  ...STARTING...
 16:58:59.059  IC705FM._vara-modem._tcp.local. can be reached at t4windoz.local.:8300 (interface 7)
 type=fm\; launchport=8273\;
```

Or on Linux

```
$ avahi-browse -r _vara-modem._tcp
+ wlp2s0 IPv6 IC705FM                                       _vara-modem._tcp   local
+ wlp2s0 IPv6 THD74FM                                       _vara-modem._tcp   local
+ wlp2s0 IPv4 THD74FM                                       _vara-modem._tcp   local
+ wlp2s0 IPv4 IC705FM                                       _vara-modem._tcp   local
= wlp2s0 IPv6 IC705FM                                       _vara-modem._tcp   local
   hostname = [t4windoz.local]
   address = [192.168.137.1]
   port = [8300]
   txt = ["type=fm;launchport=8273;"]
= wlp2s0 IPv6 THD74FM                                       _vara-modem._tcp   local
   hostname = [t4windoz.local]
   address = [192.168.137.1]
   port = [8300]
   txt = ["type=fm;launchport=8273;"]
= wlp2s0 IPv4 THD74FM                                       _vara-modem._tcp   local
   hostname = [t4windoz.local]
   address = [192.168.137.1]
   port = [8300]
   txt = ["type=fm;launchport=8273;"]
= wlp2s0 IPv4 IC705FM                                       _vara-modem._tcp   local
   hostname = [t4windoz.local]
   address = [192.168.137.1]
   port = [8300]
   txt = ["type=fm;launchport=8273;"]
```

4. Connect to `varanny` instance. Replace `t4windoz.local` with your host name returned in the above command.

```
$ telnet t4windoz.local 8273
Connected to t4windoz.local.
Escape character is '^]'.
```

5. At the prompt you can try various commands. Verify that your modems are defined properly:

```
list
OK
IC705FM
THD74
IC705HF
```

6. Start a modem and verify on your host computer that the corresponding VARA application is running and that the proper `.ini` configuration file has been applied if any was specified in your `.json` config file.

```
start IC705FM
OK
```

7. Stop the modem. The VARA application should be closed and the original `.ini` configuration restored.

```
stop
OK
Connection closed by foreign host.
```

### Logs
`varanny` logs output to the standard system logs.
