package tengo

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/go-sql-driver/mysql"
)

// DockerClientOptions specifies options when instantiating a Docker client.
// No options are currently supported, but this may change in the future.
type DockerClientOptions struct{}

// DockerClient manages lifecycle of local Docker containers for sandbox
// database instances. It wraps and hides the implementation of a specific
// Docker client implementation. (This package currently uses
// github.com/fsouza/go-dockerclient, but may later switch to the official
// Docker Golang client.)
type DockerClient struct {
	client  *docker.Client
	Options DockerClientOptions
}

// NewDockerClient is a constructor for DockerClient
func NewDockerClient(opts DockerClientOptions) (*DockerClient, error) {
	var dc *DockerClient
	client, err := docker.NewClientFromEnv()
	if err == nil {
		dc = &DockerClient{
			client:  client,
			Options: opts,
		}
	}
	return dc, err
}

// ServerArchitecture returns the architecture of the Docker engine's server,
// with values like those of runtime.GOARCH. The result is typically the same
// as runtime.GOARCH in most situations, but may differ if e.g. the binary was
// compiled as amd64 but is running on an m1 Mac via Rosetta 2.
func (dc *DockerClient) ServerArchitecture() (string, error) {
	info, err := dc.client.Info()
	if err != nil {
		return "", err
	}
	conversions := map[string]string{
		"x86_64":  "amd64",
		"aarch64": "arm64",
	}
	if converted, ok := conversions[info.Architecture]; ok {
		return converted, nil
	}
	return info.Architecture, nil
}

// DockerizedInstanceOptions specifies options for creating or finding a
// sandboxed database instance inside a Docker container.
type DockerizedInstanceOptions struct {
	Name              string
	Image             string
	RootPassword      string
	DefaultConnParams string
	DataBindMount     string // Host path to bind-mount as /var/lib/mysql in container
	CommandArgs       []string
}

// CreateInstance attempts to create a Docker container with the supplied name
// (any arbitrary name, or blank to assign random) and image (such as
// "mysql:5.6", or just "mysql" to indicate latest). A connection pool will be
// established for the instance.
func (dc *DockerClient) CreateInstance(opts DockerizedInstanceOptions) (*DockerizedInstance, error) {
	if opts.Image == "" {
		return nil, errors.New("CreateInstance: image cannot be empty string")
	}

	tokens := strings.SplitN(opts.Image, ":", 2)
	repository := tokens[0]
	tag := "latest"
	if len(tokens) > 1 {
		tag = tokens[1]
	}

	// Pull image from remote if missing
	if _, err := dc.client.InspectImage(opts.Image); err != nil {
		opts := docker.PullImageOptions{
			Repository: repository,
			Tag:        tag,
		}
		if err := dc.client.PullImage(opts, docker.AuthConfiguration{}); err != nil {
			return nil, err
		}
	}

	// Create and start container.
	// Use MYSQL_ROOT_HOST=% to ensure user root@% is created; Dockerhub "official"
	// images do this by default, but mysql/mysql-server does not. Regardless of
	// this, the HostConfig restricts connections to the host machine anyway.
	// Use LANG=C.UTF-8 to ensure client programs can pass multi-byte chars
	// correctly, since DockerizedInstance.SourceSQL shells out to the client.
	env := []string{"MYSQL_ROOT_HOST=%", "LANG=C.UTF-8"}
	if opts.RootPassword == "" {
		env = append(env, "MYSQL_ALLOW_EMPTY_PASSWORD=1")
	} else {
		env = append(env, fmt.Sprintf("MYSQL_ROOT_PASSWORD=%s", opts.RootPassword))
	}
	ccopts := docker.CreateContainerOptions{
		Name: opts.Name,
		Config: &docker.Config{
			Image: opts.Image,
			Env:   env,
			Cmd:   opts.CommandArgs,
		},
		HostConfig: &docker.HostConfig{
			PortBindings: map[docker.Port][]docker.PortBinding{
				"3306/tcp": {
					{HostIP: "127.0.0.1"},
				},
			},
		},
	}
	if opts.DataBindMount != "" {
		ccopts.HostConfig.Binds = []string{opts.DataBindMount + ":/var/lib/mysql"}
	}
	di := &DockerizedInstance{
		DockerizedInstanceOptions: opts,
		Manager:                   dc,
	}
	var err error
	if di.container, err = dc.client.CreateContainer(ccopts); err != nil {
		return nil, err
	} else if err = di.Start(); err != nil {
		return di, err
	}

	// Confirm containerized database is reachable, and create Tengo instance
	if err := di.TryConnect(); err != nil {
		return di, err
	}
	return di, nil
}

// GetInstance attempts to find an existing container with name equal to
// opts.Name. If the container is found, it will be started if not already
// running, and a connection pool will be established. If the container does
// not exist or cannot be started or connected to, a nil *DockerizedInstance
// and a non-nil error will be returned.
// If a non-blank opts.Image is supplied, and the existing container has a
// a different image, the instance's flavor will be examined as a fallback. If
// it also does not match the requested image, an error will be returned.
func (dc *DockerClient) GetInstance(opts DockerizedInstanceOptions) (*DockerizedInstance, error) {
	var err error
	di := &DockerizedInstance{
		Manager:                   dc,
		DockerizedInstanceOptions: opts,
	}
	if di.container, err = dc.client.InspectContainer(opts.Name); err != nil {
		return nil, err
	}
	actualImage := di.container.Image
	if strings.HasPrefix(actualImage, "sha256:") {
		if imageInfo, err := dc.client.InspectImage(actualImage[7:]); err == nil {
			for _, rt := range imageInfo.RepoTags {
				if rt == opts.Image || opts.Image == "" {
					actualImage = rt
					break
				}
			}
		}
	}
	if opts.Image == "" {
		di.Image = actualImage
	}
	if err = di.Start(); err != nil {
		return nil, err
	}
	if err = di.TryConnect(); err != nil {
		return nil, err
	}
	// The actual image may not match the requested one if, for example, the tag
	// for version a.b previously pointed to a.b.c but now points to a.b.d. We
	// check the instance's flavor as a fallback.
	if opts.Image != "" && opts.Image != actualImage && opts.Image != di.Flavor().String() && opts.Image != di.Flavor().Family().String() {
		return nil, fmt.Errorf("Container %s based on unexpected image: expected %s, found %s", opts.Name, opts.Image, actualImage)
	}
	return di, nil
}

// GetOrCreateInstance attempts to fetch an existing Docker container with name
// equal to opts.Name. If it exists and its image (or flavor) matches
// opts.Image, and there are no errors starting or connecting to the instance,
// it will be returned. If it exists but its image/flavor don't match, or it
// cannot be started or connected to, an error will be returned. If no container
// exists with this name, a new one will attempt to be created.
func (dc *DockerClient) GetOrCreateInstance(opts DockerizedInstanceOptions) (*DockerizedInstance, error) {
	di, err := dc.GetInstance(opts)
	if err == nil {
		return di, nil
	} else if _, ok := err.(*docker.NoSuchContainer); ok {
		return dc.CreateInstance(opts)
	}
	return nil, err
}

// DockerizedInstance is a database instance running in a local Docker
// container.
type DockerizedInstance struct {
	*Instance
	DockerizedInstanceOptions
	Manager   *DockerClient
	container *docker.Container
}

// Start starts the corresponding containerized mysql-server. If it is not
// already running, an error will be returned if it cannot be started. If it is
// already running, nil will be returned.
func (di *DockerizedInstance) Start() error {
	err := di.Manager.client.StartContainer(di.container.ID, nil)
	if _, ok := err.(*docker.ContainerAlreadyRunning); err == nil || ok {
		di.container, err = di.Manager.client.InspectContainer(di.container.ID)

		// In some cases it appears StartContainer returns prior to the port mapping
		// being in place. Retry the inspection up to 5 more times if so.
		for n := 1; err == nil && di.Port() == 0; n++ {
			if n >= 6 {
				return fmt.Errorf("Unable to find port mapping for container %s", di.Name)
			}
			time.Sleep(time.Duration(n) * time.Millisecond)
			di.container, err = di.Manager.client.InspectContainer(di.container.ID)
		}
	}
	return err
}

// Stop halts the corresponding containerized mysql-server, but does not
// destroy the container. The connection pool will be removed. If the container
// was not already running, nil will be returned.
func (di *DockerizedInstance) Stop() error {
	di.CloseAll()
	err := di.Manager.client.StopContainer(di.container.ID, 10)
	if _, ok := err.(*docker.ContainerNotRunning); !ok && err != nil {
		return err
	}
	return nil
}

// Destroy stops and deletes the corresponding containerized mysql-server.
func (di *DockerizedInstance) Destroy() error {
	di.CloseAll()
	rcopts := docker.RemoveContainerOptions{
		ID:            di.container.ID,
		Force:         true,
		RemoveVolumes: true,
	}
	err := di.Manager.client.RemoveContainer(rcopts)
	if _, ok := err.(*docker.NoSuchContainer); ok {
		err = nil
	}
	return err
}

// TryConnect sets up a connection pool to the containerized mysql-server,
// and tests connectivity. It returns an error if a connection cannot be
// established within 30 seconds.
func (di *DockerizedInstance) TryConnect() (err error) {
	var ok bool
	di.Instance, err = NewInstance("mysql", di.DSN())
	if err != nil {
		return err
	}
	maxAttempts := 120
	if di.DataBindMount != "" { // bind mounted dir causes slower startup
		maxAttempts *= 2
	}
	for attempts := 0; attempts < maxAttempts; attempts++ {
		if ok, err = di.Instance.CanConnect(); ok {
			return err
		}
		time.Sleep(250 * time.Millisecond)
	}
	return err
}

// Port returns the actual port number on localhost that maps to the container's
// internal port 3306.
func (di *DockerizedInstance) Port() int {
	return di.PortMap(3306)
}

// PortMap returns the port number on localhost that maps to the container's
// specified internal tcp port.
func (di *DockerizedInstance) PortMap(containerPort int) int {
	portAndProto := docker.Port(fmt.Sprintf("%d/tcp", containerPort))
	portBindings, ok := di.container.NetworkSettings.Ports[portAndProto]
	if !ok || len(portBindings) == 0 {
		return 0
	}
	result, _ := strconv.Atoi(portBindings[0].HostPort)
	return result
}

// DSN returns a github.com/go-sql-driver/mysql formatted DSN corresponding
// to its containerized mysql-server instance.
func (di *DockerizedInstance) DSN() string {
	var pass string
	if di.RootPassword != "" {
		pass = fmt.Sprintf(":%s", di.RootPassword)
	}
	return fmt.Sprintf("root%s@tcp(127.0.0.1:%d)/?%s", pass, di.Port(), di.DefaultConnParams)
}

func (di *DockerizedInstance) String() string {
	return fmt.Sprintf("DockerizedInstance:%d", di.Port())
}

// NukeData drops all non-system schemas and tables in the containerized
// mysql-server, making it useful as a per-test cleanup method in
// implementations of IntegrationTestSuite.BeforeTest. This method should
// never be used on a "real" production database!
func (di *DockerizedInstance) NukeData() error {
	schemas, err := di.Instance.SchemaNames()
	if err != nil {
		return err
	}
	db, err := di.Instance.CachedConnectionPool("", "")
	if err != nil {
		return err
	}
	var retries []string
	for _, schema := range schemas {
		// Just run a DROP DATABASE directly, without dropping tables first. This is
		// not safe in prod, but fine for tests.
		if err := dropSchema(db, schema); err != nil {
			retries = append(retries, schema)
		}
	}

	// Retry failures once, this time using a connection pool with
	// foreign_key_checks disabled, in case the issue was cross-DB FKs. (This is
	// rare, and we generally already had a pool without that set, which is why
	// we don't use it from the start.)
	if len(retries) > 0 {
		db, err := di.Instance.ConnectionPool("", "foreign_key_checks=0")
		if err != nil {
			return err
		}
		defer db.Close()
		for _, schema := range retries {
			if err := dropSchema(db, schema); err != nil {
				return err
			}
		}
	}

	// Close all schema-specific cached connection pools. Cache key format is
	// "schema?params", so any key not beginning with ? is schema-specific.
	di.Instance.m.Lock()
	defer di.Instance.m.Unlock()
	for key, connPool := range di.Instance.connectionPool {
		if len(key) > 0 && key[0] != '?' {
			connPool.Close()
			delete(di.Instance.connectionPool, key)
		}
	}

	return nil
}

// SourceSQL reads the specified file and executes it against the containerized
// mysql-server. The file should contain one or more valid SQL instructions,
// typically a mix of DML and/or DDL statements. It is useful as a per-test
// setup method in implementations of IntegrationTestSuite.BeforeTest.
func (di *DockerizedInstance) SourceSQL(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("SourceSQL %s: Unable to open setup file %s: %s", di, filePath, err)
	}
	defer f.Close()
	cmd := []string{"mysql", "-tvvv", "-u", "root", "-h", "127.0.0.1", "--default-character-set", "utf8mb4"}
	if di.Flavor().Min(FlavorMariaDB110) {
		cmd[0] = "mariadb" // MariaDB 11.0+ images don't include `mysql` symlink
	}
	if di.RootPassword != "" {
		cmd = append(cmd, fmt.Sprintf("-p%s", di.RootPassword))
	}
	stdoutStr, stderrStr, err := di.Exec(cmd, f)
	stderrStr = strings.Replace(stderrStr, "Warning: Using a password on the command line interface can be insecure.\n", "", 1)
	if err != nil || strings.Contains(stderrStr, "ERROR") {
		err = fmt.Errorf("SourceSQL %s: Error sourcing file %s: %s", di, filePath, stderrStr)
	}
	return stdoutStr, err
}

// Exec executes the supplied command/args in the container, blocks until
// completion, and returns STDOUT and STDERR. An input stream may optionally
// be supplied for the exec's STDIN.
func (di *DockerizedInstance) Exec(cmd []string, stdin io.Reader) (string, string, error) {
	ceopts := docker.CreateExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		AttachStdin:  true,
		Cmd:          cmd,
		Container:    di.container.ID,
	}
	exec, err := di.Manager.client.CreateExec(ceopts)
	if err != nil {
		return "", "", err
	}
	var stdout, stderr bytes.Buffer
	seopts := docker.StartExecOptions{
		OutputStream: &stdout,
		ErrorStream:  &stderr,
		InputStream:  stdin,
	}
	if err = di.Manager.client.StartExec(exec.ID, seopts); err != nil {
		return "", "", err
	}
	execInfo, err := di.Manager.client.InspectExec(exec.ID)
	if err == nil && execInfo.ExitCode > 0 {
		err = fmt.Errorf("Process returned exit code %d", execInfo.ExitCode)
	}
	return stdout.String(), stderr.String(), err
}

// ContainerNameForImage returns a usable container name (or portion of a name)
// based on the supplied image name.
func ContainerNameForImage(image string) string {
	if strings.Contains(image, "/") {
		image = strings.Replace(image, "/mysql-server", "", 1)
		image = strings.ReplaceAll(image, "/", "-")
	}
	return strings.ReplaceAll(image, ":", "-")
}

type filteredLogger struct {
	logger *log.Logger
}

func (fl filteredLogger) Print(v ...interface{}) {
	for _, arg := range v {
		if err, ok := arg.(error); ok {
			if msg := err.Error(); strings.Contains(msg, "EOF") || strings.Contains(msg, "unexpected read") {
				return
			}
		}
	}
	fl.logger.Print(v...)
}

// UseFilteredDriverLogger overrides the mysql driver's logger to avoid excessive
// messages. This suppresses the driver's "unexpected EOF" output, which occurs
// when an initial connection is refused or a connection drops early. This
// excessive logging can occur whenever DockerClient.CreateInstance() or
// DockerClient.GetInstance() is waiting for the instance to finish starting.
func UseFilteredDriverLogger() {
	fl := filteredLogger{
		logger: log.New(os.Stderr, "[mysql] ", log.Ldate|log.Ltime|log.Lshortfile),
	}
	mysql.SetLogger(fl)
}
