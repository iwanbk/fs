package cache

import (
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"net/url"
	"fmt"
	"os"
	"path"
	"io/ioutil"
	"io"
	"path/filepath"
	"bufio"
	"os/user"
	"strings"
)

type sftpCache struct {
	url string
	client *sftp.Client
	root string
	dedupe string
}

func loadPrivateKeys() ssh.AuthMethod {
	home := os.Getenv("HOME")
	sshDir := path.Join(home, ".ssh")
	signers := make([]ssh.Signer, 0)

	//Identity files lookup order is as defined by ssh manual
	for _, key := range []string{"id_dsa", "id_ecdsa", "id_ed25519", "id_rsa"} {
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
			log.Error("Failed to parse identity file '%s': %s", identityFilPath, err)
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
		url: URL,
		client: sftpClient,
		root: u.Path,
		dedupe: dedupe,
	}, nil
}

func (c *sftpCache) String() string {
	return c.url
}

func (c *sftpCache) Open(path string) (io.ReadSeeker, error) {
	chrootPath := chroot(c.root, filepath.Join(c.dedupe, "files", path))
	return os.Open(chrootPath)
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