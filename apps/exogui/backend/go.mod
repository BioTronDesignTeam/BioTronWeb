module github.com/BioTronDesignTeam/BioTronWeb/apps/exogui/backend

go 1.27.3

require (
	github.com/BioTronDesignTeam/BioTronWeb/go/authclient v0.0.0
	github.com/BioTronDesignTeam/BioTronWeb/go/logclient v0.0.0
	github.com/joho/godotenv v1.5.1
)

replace github.com/BioTronDesignTeam/BioTronWeb/go/authclient => ../../../go/authclient

replace github.com/BioTronDesignTeam/BioTronWeb/go/logclient => ../../../go/logclient
