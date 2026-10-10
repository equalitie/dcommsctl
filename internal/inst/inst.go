package inst

import (
	"fmt"
	"html/template"
	"math/rand/v2"
	"os"
	"path"
	"path/filepath"

	"github.com/joho/godotenv"
)

type InstanceGenericConfig struct {
	SSOIssuerURI string
	InstanceID   string
	ClientID     string
	ClientSecret string
	Domain       string
	OnionDomain  string
	Version      string
	// we support ports via that for now
	Env map[string]string
}

func instanceGenID() string {
	return instanceNames[rand.IntN(len(instanceNames))]
}

func InstanceCreate(instancesDir string, instanceName string) (string, error) {
	instanceID := instanceName + "-" + instanceGenID()

	pathToInst := path.Join(instancesDir, instanceID)
	for {
		if _, err := os.Stat(pathToInst); err != nil {
			if os.IsNotExist(err) {
				break
			}
		}

		instanceID = instanceName + "-" + instanceGenID()
		pathToInst = path.Join(instancesDir, instanceID)
	}

	folderToCreate := path.Join(pathToInst, "jobs")

	if err := os.MkdirAll(folderToCreate, 0o700); err != nil {
		return "", fmt.Errorf("create instance dir: %s", err)
	}

	return pathToInst, nil
}

func InstanceSetup(appName, instancesDir string, config InstanceGenericConfig) (string, error) {
	pathToApp := path.Join("conf_tmpl", appName)
	pathToInst, err := InstanceCreate(instancesDir, appName)
	if err != nil {
		return "", err
	}

	instanceID := filepath.Base(pathToInst)

	config.InstanceID = instanceID

	if err := WriteFileTemplate(
		path.Join(pathToApp, "docker-compose.tmpl.yml"),
		path.Join(pathToInst, "docker-compose.yml"),
		&config,
	); err != nil {
		return "", err
	}

	envFilePath := path.Join(pathToInst, ".env")

	if err := WriteFileTemplate(
		path.Join(pathToApp, "env.tmpl"),
		envFilePath,
		&config,
	); err != nil {
		return "", err
	}

	appendEnv, err := godotenv.Marshal(config.Env)
	if err != nil {
		return "", err
	}

	f, err := os.OpenFile(envFilePath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Leading newline handles files without a trailing newline.
	if _, err := f.WriteString("\n" + appendEnv + "\n"); err != nil {
		return "", err
	}
	if err := WriteFileTemplate(
		path.Join(pathToApp, "caddy-route.tmpl.json"),
		path.Join(pathToInst, "caddy-route.json"),
		&config,
	); err != nil {
		return "", err
	}

	return pathToInst, nil
}

func WriteFileTemplate(tmplPath, targetPath string, data any) error {
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		return err
	}

	composeFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	if err := tmpl.Execute(composeFile, &data); err != nil {
		return err
	}

	return nil
}

var AppIDs = []string{
	"synapse",
	"cryptpad",
	"peertube",
	"mastodon",
	"ersatz",
}

var instanceNames = []string{
	"aardwolf",
	"axolotl",
	"babirusa",
	"bilby",
	"binturong",
	"bongo",
	"caracal",
	"coati",
	"dhole",
	"dikdik",
	"dugong",
	"echidna",
	"fossa",
	"gerenuk",
	"genet",
	"goura",
	"hirola",
	"indri",
	"jerboa",
	"kakapo",
	"kinkajou",
	"klipspringer",
	"kudu",
	"linsang",
	"margay",
	"markhor",
	"numbat",
	"okapi",
	"onager",
	"olingo",
	"pademelon",
	"pangolin",
	"pika",
	"quokka",
	"quoll",
	"saiga",
	"serval",
	"solenodon",
	"takin",
	"tamandua",
	"tarsier",
	"tenrec",
	"tuatara",
	"uakari",
	"vaquita",
	"wallaroo",
	"xerus",
	"yapok",
	"zorilla",
	"agouti",
	"anoa",
	"bharal",
	"cacomistle",
	"cuscus",
	"galago",
	"iriomote",
	"jentink",
	"kiang",
	"kouprey",
	"manul",
	"muntjac",
	"nyala",
	"oryx",
	"potto",
	"sifaka",
	"tahr",
	"vicuna",
	"viscacha",
	"zebu",
	"zokor",
}
