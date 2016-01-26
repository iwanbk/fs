package cache

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strings"

	"github.com/Jumpscale/aysfs/utils"
	"github.com/dsnet/compress/brotli"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type sftpCache struct {
	url    string
	client *sftp.Client
	root   string
	dedupe string
}

func loadPrivateKeys() ssh.AuthMethod {
	home := os.Getenv("HOME")
	sshDir := path.Join(home, ".ssh")
	signers := make([]ssh.Signer, 0)

	//Identity files lookup order is as defined by ssh manual
	f, err := os.Open(sshDir)
	defer f.Close()
	if err != nil {
		log.Fatal("Error openin ssh dir: %s", err)
	}
	names, err := f.Readdirnames(-1)
	if err != nil {
		log.Fatal("Error openin ssh dir: %s", err)
	}

	for _, key := range names {
		identityFilPath := path.Join(sshDir, key)
		if _, err := os.Stat(identityFilPath); os.IsNotExist(err) {
			continue
		}
		file, err := os.Open(identityFilPath)
		if err != nil {
			log.Error("Failed to open identity file '%s': %s", identityFilPath, err)
			continue
		}
		defer file.Close()
		content, err := ioutil.ReadAll(file)
		if err != nil {
			log.Error("Failed to read identity file '%s': %s", identityFilPath, err)
			continue
		}
		signer, err := ssh.ParsePrivateKey(content)
		if err != nil {
			log.Debug("Failed to parse identity file '%s': %s", identityFilPath, err)
			continue
		}

		signers = append(signers, signer)
	}

	return ssh.PublicKeys(signers...)
}

func NewSFTPCache(URL string, dedupe string) (Cache, error) {
	u, err := url.Parse(URL)
	if err != nil {
		return nil, err
	}

	if u.Scheme != "ssh" {
		return nil, fmt.Errorf("Invalid sftp schema '%s' expecting ssh", u.Scheme)
	}

	var config ssh.ClientConfig

	if u.User != nil {
		config.User = u.User.Username()
		if password, ok := u.User.Password(); ok {
			config.Auth = append(config.Auth, ssh.Password(password))
		}
	} else {
		user, err := user.Current()
		if err != nil {
			return nil, err
		}
		config.User = user.Name
	}

	config.Auth = append(config.Auth, loadPrivateKeys())
	host := u.Host
	if strings.Index(u.Host, ":") < 0 {
		host = fmt.Sprintf("%s:22", host)
	}

	sshClient, err := ssh.Dial("tcp", host, &config)
	if err != nil {
		return nil, err
	}

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return nil, err
	}

	return &sftpCache{
		url:    URL,
		client: sftpClient,
		root:   u.Path,
		dedupe: dedupe,
	}, nil
}

func (c *sftpCache) String() string {
	return c.url
}

func (c *sftpCache) Open(path string) (io.ReadSeeker, error) {
	chrootPath := chroot(c.root, filepath.Join(c.dedupe, path+".bro"))

	var r io.ReadSeeker
	var err error
	r, err = c.client.Open(chrootPath) // try compressed file
	if err != nil {
		log.Errorf("Can't open file %s", chrootPath)
		return nil, err
	}

	log.Debugf("Open compressed file %s", chrootPath)
	return utils.NewReadSeeker(brotli.NewReader(r)), nil
}

func (c *sftpCache) GetMetaData(id string) ([]string, error) {
	path := filepath.Join(c.dedupe, "md", fmt.Sprintf("%s.flist", id))
	chrootPath := chroot(c.root, path)
	file, err := c.client.Open(chrootPath)
	if err != nil {
		return nil, err
	}

	metadata := []string{}
	scanner := bufio.NewScanner(file)
	var line string
	for scanner.Scan() {
		line = scanner.Text()
		if err := scanner.Err(); err != nil {
			return nil, err
		}

		metadata = append(metadata, line)
	}

	return metadata, nil
}

func (c *sftpCache) Exists(path string) bool {
	_, err := c.client.Stat(path)
	return os.IsExist(err)
}

func (c *sftpCache) BasePath() string {
	return path.Join(c.root, c.dedupe)
}
