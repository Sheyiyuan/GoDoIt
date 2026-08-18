module github.com/Sheyiyuan/GoDoIt/cli

go 1.24.0

require (
	github.com/AlecAivazis/survey/v2 v2.3.7
	github.com/Sheyiyuan/GoDoIt/core v0.0.0
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211
)

require (
	github.com/BurntSushi/toml v1.4.0 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/mattn/go-colorable v0.1.2 // indirect
	github.com/mattn/go-isatty v0.0.8 // indirect
	github.com/mgutz/ansi v0.0.0-20170206155736-9520e82c474b // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.4.0 // indirect
)

replace github.com/Sheyiyuan/GoDoIt/core => ../core
