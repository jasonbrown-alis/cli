module github.com/alis-x/cli/alis

go 1.17

// Use the replace when developing locally
//replace go.protobuf.alis.dev => ../../alis/protobuf/go/alis

//replace go.lib.alis.dev => ../../alis/os1/os-go
//replace go.protobuf.alis.alis.exchange => ../../alis.exchange/alis/protobuf/go

require (
	github.com/mitchellh/go-homedir v1.1.0
	github.com/pterm/pterm v0.12.40
	github.com/spf13/cobra v1.4.0
	github.com/spf13/viper v1.10.1
	go.protobuf.alis.alis.exchange v0.0.0-20220410111307-fefa023d3e9e
	golang.org/x/oauth2 v0.0.0-20220309155454-6242fa91716a
	google.golang.org/api v0.63.0
	google.golang.org/genproto v0.0.0-20220407144326-9054f6ed7bac
	google.golang.org/grpc v1.45.0
	google.golang.org/protobuf v1.28.0
)

require (
	cloud.google.com/go v0.99.0 // indirect
	github.com/atomicgo/cursor v0.0.1 // indirect
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/gookit/color v1.5.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/kr/pretty v0.2.0 // indirect
	github.com/magiconair/properties v1.8.5 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/mitchellh/mapstructure v1.4.3 // indirect
	github.com/pelletier/go-toml v1.9.4 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/spf13/afero v1.6.0 // indirect
	github.com/spf13/cast v1.4.1 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/subosito/gotenv v1.2.0 // indirect
	github.com/xo/terminfo v0.0.0-20210125001918-ca9a967f8778 // indirect
	go.opencensus.io v0.23.0 // indirect
	golang.org/x/net v0.0.0-20220127200216-cd36cc0744dd // indirect
	golang.org/x/sys v0.0.0-20220319134239-a9b59b0215f8 // indirect
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211 // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/ini.v1 v1.66.2 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)
