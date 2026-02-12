// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var (
	connectServer           string
	connectToken            string
	connectUsername         string
	connectPassword         string
	connectClientCert       string
	connectClientKey        string
	connectContextName      string
	connectClusterName      string
	connectUserName         string
	connectNamespace        string
	connectCAFile           string
	connectInsecure         bool
	connectSourceKubeconfig string
	connectSourceContext    string
	connectKubeconfigPath   string
	connectSkipVerify       bool
	connectLaunchMap        bool
)

var connectCmd = &cobra.Command{
	Use:   "connect [server-url]",
	Short: "Connect to a Kubernetes cluster and start exploring",
	Long: `Create or import a kubeconfig context for quick Cub Scout usage.

You can connect in two ways:
  1. Direct server mode: provide a Kubernetes API server URL plus auth flags.
  2. Kubeconfig import mode: copy a context from another kubeconfig file.

Examples:
  # Direct connect with bearer token
  cub-scout connect https://api.example.com:6443 --token "$K8S_BEARER_TOKEN" --context prod --go

  # Direct connect with client cert auth
  cub-scout connect api.example.com:6443 \
    --client-cert ~/.kube/client.crt \
    --client-key ~/.kube/client.key \
    --context prod

  # Import an existing context from shared kubeconfig and launch map immediately
  cub-scout connect --from-kubeconfig ./artem.yaml --from-context ske-vcl-pro --go
`,
	Args: cobra.MaximumNArgs(1),
	RunE: runConnect,
}

func init() {
	connectCmd.Flags().StringVar(&connectServer, "server", "", "Kubernetes API server URL (https://host:6443). If omitted, first argument is used.")
	connectCmd.Flags().StringVar(&connectToken, "token", "", "Bearer token for authentication (or set K8S_BEARER_TOKEN)")
	connectCmd.Flags().StringVar(&connectUsername, "username", "", "Username for basic auth")
	connectCmd.Flags().StringVar(&connectPassword, "password", "", "Password for basic auth")
	connectCmd.Flags().StringVar(&connectClientCert, "client-cert", "", "Client certificate file path for TLS auth")
	connectCmd.Flags().StringVar(&connectClientKey, "client-key", "", "Client key file path for TLS auth")
	connectCmd.Flags().StringVar(&connectContextName, "context", "", "Context name to create (default: derived from server/selected context)")
	connectCmd.Flags().StringVar(&connectClusterName, "cluster", "", "Cluster entry name (default: <context>-cluster)")
	connectCmd.Flags().StringVar(&connectUserName, "user", "", "User entry name (default: <context>-user)")
	connectCmd.Flags().StringVarP(&connectNamespace, "namespace", "n", "default", "Default namespace for the context")
	connectCmd.Flags().StringVar(&connectCAFile, "ca-file", "", "Certificate authority file path")
	connectCmd.Flags().BoolVar(&connectInsecure, "insecure", false, "Skip TLS verification")
	connectCmd.Flags().StringVar(&connectSourceKubeconfig, "from-kubeconfig", "", "Import context from this kubeconfig file")
	connectCmd.Flags().StringVar(&connectSourceContext, "from-context", "", "Context name inside --from-kubeconfig to import")
	connectCmd.Flags().StringVar(&connectKubeconfigPath, "kubeconfig", "", "Destination kubeconfig path (default: first KUBECONFIG entry or ~/.kube/config)")
	connectCmd.Flags().BoolVar(&connectSkipVerify, "skip-verify", false, "Skip Kubernetes API connectivity check")
	connectCmd.Flags().BoolVar(&connectLaunchMap, "go", false, "Launch cub-scout map immediately after connect")

	rootCmd.AddCommand(connectCmd)
}

func runConnect(cmd *cobra.Command, args []string) error {
	if connectSourceKubeconfig != "" {
		if connectServer != "" || len(args) > 0 {
			return fmt.Errorf("cannot use --server or [server-url] with --from-kubeconfig")
		}
		return runConnectFromKubeconfig()
	}

	if len(args) > 0 && connectServer == "" {
		connectServer = args[0]
	}
	if connectServer == "" {
		return fmt.Errorf("missing server URL; pass [server-url], --server, or use --from-kubeconfig")
	}
	return runConnectFromServer()
}

func runConnectFromKubeconfig() error {
	srcPath := expandTilde(connectSourceKubeconfig)
	srcCfg, err := clientcmd.LoadFromFile(srcPath)
	if err != nil {
		return fmt.Errorf("load source kubeconfig %q: %w", srcPath, err)
	}

	srcCtxName, err := selectSourceContext(srcCfg, connectSourceContext)
	if err != nil {
		return err
	}
	srcCtx := srcCfg.Contexts[srcCtxName]
	if srcCtx == nil {
		return fmt.Errorf("source context %q not found", srcCtxName)
	}
	srcCluster := srcCfg.Clusters[srcCtx.Cluster]
	if srcCluster == nil {
		return fmt.Errorf("source context %q references missing cluster %q", srcCtxName, srcCtx.Cluster)
	}
	srcAuth := srcCfg.AuthInfos[srcCtx.AuthInfo]
	if srcAuth == nil {
		return fmt.Errorf("source context %q references missing user %q", srcCtxName, srcCtx.AuthInfo)
	}

	destPath, destCfg, err := loadDestinationKubeconfig(connectKubeconfigPath)
	if err != nil {
		return err
	}

	contextName := connectContextName
	if contextName == "" {
		contextName = sanitizeContextName(srcCtxName)
	}
	if contextName == "" {
		contextName = "cluster"
	}

	clusterName := connectClusterName
	if clusterName == "" {
		clusterName = contextName + "-cluster"
	}
	userName := connectUserName
	if userName == "" {
		userName = contextName + "-user"
	}

	namespace := connectNamespace
	if namespace == "" {
		namespace = srcCtx.Namespace
	}
	if namespace == "" {
		namespace = "default"
	}

	if destCfg.Clusters == nil {
		destCfg.Clusters = map[string]*clientcmdapi.Cluster{}
	}
	if destCfg.AuthInfos == nil {
		destCfg.AuthInfos = map[string]*clientcmdapi.AuthInfo{}
	}
	if destCfg.Contexts == nil {
		destCfg.Contexts = map[string]*clientcmdapi.Context{}
	}

	destCfg.Clusters[clusterName] = srcCluster.DeepCopy()
	destCfg.AuthInfos[userName] = srcAuth.DeepCopy()
	destCfg.Contexts[contextName] = &clientcmdapi.Context{
		Cluster:   clusterName,
		AuthInfo:  userName,
		Namespace: namespace,
	}
	destCfg.CurrentContext = contextName

	if err := writeKubeconfig(destPath, destCfg); err != nil {
		return err
	}

	fmt.Printf("✓ Imported context %q from %s\n", srcCtxName, srcPath)
	printConnectSummary(contextName, destPath, connectSkipVerify)

	if !connectSkipVerify {
		version, verifyErr := verifyContext(destPath, contextName)
		if verifyErr != nil {
			return fmt.Errorf("context saved but API check failed: %w\nre-run with --skip-verify if the endpoint is temporarily unreachable", verifyErr)
		}
		fmt.Printf("✓ Kubernetes API reachable (%s)\n", version)
	}

	if connectLaunchMap {
		return launchMap(destPath)
	}
	fmt.Println("Next: cub-scout map")
	return nil
}

func runConnectFromServer() error {
	serverURL, err := normalizeServerURL(connectServer)
	if err != nil {
		return err
	}
	if connectInsecure && connectCAFile != "" {
		return fmt.Errorf("cannot use --insecure and --ca-file together")
	}

	token := connectToken
	if token == "" {
		token = os.Getenv("K8S_BEARER_TOKEN")
	}

	authInfo, authLabel, err := buildAuthInfo(token, connectUsername, connectPassword, connectClientCert, connectClientKey)
	if err != nil {
		return err
	}

	destPath, destCfg, err := loadDestinationKubeconfig(connectKubeconfigPath)
	if err != nil {
		return err
	}

	contextName := connectContextName
	if contextName == "" {
		contextName = defaultContextName(serverURL)
	}
	clusterName := connectClusterName
	if clusterName == "" {
		clusterName = contextName + "-cluster"
	}
	userName := connectUserName
	if userName == "" {
		userName = contextName + "-user"
	}

	cluster := &clientcmdapi.Cluster{
		Server:                serverURL,
		InsecureSkipTLSVerify: connectInsecure,
	}
	if connectCAFile != "" {
		cluster.CertificateAuthority = expandTilde(connectCAFile)
	}

	if destCfg.Clusters == nil {
		destCfg.Clusters = map[string]*clientcmdapi.Cluster{}
	}
	if destCfg.AuthInfos == nil {
		destCfg.AuthInfos = map[string]*clientcmdapi.AuthInfo{}
	}
	if destCfg.Contexts == nil {
		destCfg.Contexts = map[string]*clientcmdapi.Context{}
	}

	destCfg.Clusters[clusterName] = cluster
	destCfg.AuthInfos[userName] = authInfo
	destCfg.Contexts[contextName] = &clientcmdapi.Context{
		Cluster:   clusterName,
		AuthInfo:  userName,
		Namespace: connectNamespace,
	}
	destCfg.CurrentContext = contextName

	if err := writeKubeconfig(destPath, destCfg); err != nil {
		return err
	}

	fmt.Printf("✓ Created context %q (%s)\n", contextName, authLabel)
	printConnectSummary(contextName, destPath, connectSkipVerify)

	if !connectSkipVerify {
		version, verifyErr := verifyContext(destPath, contextName)
		if verifyErr != nil {
			return fmt.Errorf("context saved but API check failed: %w\nre-run with --skip-verify if the endpoint is temporarily unreachable", verifyErr)
		}
		fmt.Printf("✓ Kubernetes API reachable (%s)\n", version)
	}

	if connectLaunchMap {
		return launchMap(destPath)
	}
	fmt.Println("Next: cub-scout map")
	return nil
}

func buildAuthInfo(token, username, password, clientCert, clientKey string) (*clientcmdapi.AuthInfo, string, error) {
	modes := 0
	if token != "" {
		modes++
	}
	if username != "" || password != "" {
		modes++
	}
	if clientCert != "" || clientKey != "" {
		modes++
	}
	if modes > 1 {
		return nil, "", fmt.Errorf("choose exactly one auth mode: token, username/password, or client-cert/client-key")
	}
	if (username == "") != (password == "") {
		return nil, "", fmt.Errorf("both --username and --password are required for basic auth")
	}
	if (clientCert == "") != (clientKey == "") {
		return nil, "", fmt.Errorf("both --client-cert and --client-key are required for TLS client auth")
	}

	auth := &clientcmdapi.AuthInfo{}
	if token != "" {
		auth.Token = token
		return auth, "bearer token", nil
	}
	if username != "" {
		auth.Username = username
		auth.Password = password
		return auth, "basic auth", nil
	}
	if clientCert != "" {
		auth.ClientCertificate = expandTilde(clientCert)
		auth.ClientKey = expandTilde(clientKey)
		return auth, "client cert", nil
	}
	return auth, "no auth", nil
}

func selectSourceContext(cfg *clientcmdapi.Config, preferred string) (string, error) {
	if preferred != "" {
		if _, ok := cfg.Contexts[preferred]; !ok {
			return "", fmt.Errorf("context %q not found in source kubeconfig", preferred)
		}
		return preferred, nil
	}
	if cfg.CurrentContext != "" {
		if _, ok := cfg.Contexts[cfg.CurrentContext]; ok {
			return cfg.CurrentContext, nil
		}
	}
	if len(cfg.Contexts) == 1 {
		for name := range cfg.Contexts {
			return name, nil
		}
	}

	var names []string
	for name := range cfg.Contexts {
		names = append(names, name)
	}
	return "", fmt.Errorf("source kubeconfig has multiple contexts; pass --from-context (%s)", strings.Join(names, ", "))
}

func normalizeServerURL(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", fmt.Errorf("server URL cannot be empty")
	}
	if !strings.Contains(v, "://") {
		v = "https://" + v
	}

	u, err := url.Parse(v)
	if err != nil {
		return "", fmt.Errorf("invalid server URL %q: %w", raw, err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("invalid server URL %q: missing host", raw)
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	return u.String(), nil
}

func defaultContextName(serverURL string) string {
	u, err := url.Parse(serverURL)
	if err != nil || u.Host == "" {
		return "cluster"
	}
	host := strings.Split(u.Host, ":")[0]
	name := sanitizeContextName(host)
	if name == "" {
		return "cluster"
	}
	return name
}

func sanitizeContextName(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(v) {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlnum {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	return out
}

func loadDestinationKubeconfig(pathFlag string) (string, *clientcmdapi.Config, error) {
	path := defaultKubeconfigPath()
	if pathFlag != "" {
		path = expandTilde(pathFlag)
	}

	cfg, err := clientcmd.LoadFromFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return path, clientcmdapi.NewConfig(), nil
		}
		return "", nil, fmt.Errorf("load kubeconfig %q: %w", path, err)
	}
	return path, cfg, nil
}

func writeKubeconfig(path string, cfg *clientcmdapi.Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create kubeconfig directory %q: %w", dir, err)
	}
	if err := clientcmd.WriteToFile(*cfg, path); err != nil {
		return fmt.Errorf("write kubeconfig %q: %w", path, err)
	}
	return nil
}

func verifyContext(kubeconfigPath, contextName string) (string, error) {
	loading := clientcmd.NewDefaultClientConfigLoadingRules()
	loading.ExplicitPath = kubeconfigPath
	overrides := &clientcmd.ConfigOverrides{CurrentContext: contextName}

	restCfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loading, overrides).ClientConfig()
	if err != nil {
		return "", err
	}
	restCfg.Timeout = 7 * time.Second

	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return "", err
	}

	version, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return "", err
	}
	if version != nil && version.GitVersion != "" {
		return version.GitVersion, nil
	}
	return "unknown version", nil
}

func printConnectSummary(contextName, kubeconfigPath string, skipVerify bool) {
	fmt.Printf("✓ Current context set to %q\n", contextName)
	fmt.Printf("✓ Kubeconfig updated: %s\n", kubeconfigPath)
	if skipVerify {
		fmt.Println("• Connectivity check skipped (--skip-verify)")
	}
}

func launchMap(kubeconfigPath string) error {
	fmt.Println("Launching: cub-scout map")
	cmd := exec.Command(os.Args[0], "map")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = setOrReplaceEnv(os.Environ(), "KUBECONFIG", kubeconfigPath)
	return cmd.Run()
}

func setOrReplaceEnv(env []string, key, value string) []string {
	prefix := key + "="
	updated := make([]string, 0, len(env)+1)
	replaced := false
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			if !replaced {
				updated = append(updated, prefix+value)
				replaced = true
			}
			continue
		}
		updated = append(updated, entry)
	}
	if !replaced {
		updated = append(updated, prefix+value)
	}
	return updated
}

func defaultKubeconfigPath() string {
	if env := os.Getenv("KUBECONFIG"); env != "" {
		parts := strings.Split(env, string(os.PathListSeparator))
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			return expandTilde(parts[0])
		}
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".kube/config"
	}
	return filepath.Join(home, ".kube", "config")
}

func expandTilde(path string) string {
	if path == "" {
		return path
	}
	if path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
