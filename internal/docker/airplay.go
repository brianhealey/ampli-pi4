package docker

import (
	"context"
	"fmt"
	"net"
	"net/netip"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

// AirPlayManager manages AirPlay container lifecycle
type AirPlayManager struct {
	cli              *client.Client
	image            string
	macvlanNetwork   string
	internalNetwork  string
	configVolume     string
	ipRangeStart     string // e.g., "10.100.10.98"
	nextIPOffset     int
}

// NewAirPlayManager creates a new AirPlay container manager
func NewAirPlayManager(image, macvlanNet, internalNet, configVol, ipStart string) (*AirPlayManager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &AirPlayManager{
		cli:              cli,
		image:            image,
		macvlanNetwork:   macvlanNet,
		internalNetwork:  internalNet,
		configVolume:     configVol,
		ipRangeStart:     ipStart,
		nextIPOffset:     0,
	}, nil
}

// CreateContainer creates a new AirPlay container for the given stream
func (m *AirPlayManager) CreateContainer(ctx context.Context, streamID int, streamName, alsaDevice string) (string, error) {
	containerName := fmt.Sprintf("airplay-%d", streamID)
	hostname := fmt.Sprintf("amplipi-stream-%d", streamID)

	// Allocate an IP address
	ipStr := m.allocateIP()
	ipAddr, err := netip.ParseAddr(ipStr)
	if err != nil {
		return "", fmt.Errorf("invalid IP address %s: %w", ipStr, err)
	}

	// Container configuration
	config := &container.Config{
		Image:    m.image,
		Hostname: hostname,
		Env: []string{
			fmt.Sprintf("STREAM_ID=%d", streamID),
			fmt.Sprintf("AIRPLAY_NAME=%s", streamName),
			fmt.Sprintf("ALSA_DEVICE=%s", alsaDevice),
			"AIRPLAY2_ENABLED=true",
		},
	}

	// Host configuration
	hostConfig := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyUnlessStopped,
		},
		Resources: container.Resources{
			Devices: []container.DeviceMapping{
				{
					PathOnHost:        "/dev/snd",
					PathInContainer:   "/dev/snd",
					CgroupPermissions: "rwm",
				},
			},
		},
		Mounts: []mount.Mount{
			{
				Type:     mount.TypeVolume,
				Source:   m.configVolume,
				Target:   "/config",
				ReadOnly: true,
			},
		},
		GroupAdd: []string{"29"}, // audio group
	}

	// Network configuration - use netip.Addr for IP
	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			m.macvlanNetwork: {
				IPAMConfig: &network.EndpointIPAMConfig{
					IPv4Address: ipAddr,
				},
			},
			m.internalNetwork: {},
		},
	}

	// Create the container
	opts := client.ContainerCreateOptions{
		Name:             containerName,
		Config:           config,
		HostConfig:       hostConfig,
		NetworkingConfig: networkConfig,
	}

	resp, err := m.cli.ContainerCreate(ctx, opts)
	if err != nil {
		return "", fmt.Errorf("failed to create container %s: %w", containerName, err)
	}

	// Start the container
	startOpts := client.ContainerStartOptions{}
	if _, err := m.cli.ContainerStart(ctx, resp.ID, startOpts); err != nil {
		// Clean up on failure
		rmOpts := client.ContainerRemoveOptions{Force: true}
		m.cli.ContainerRemove(ctx, resp.ID, rmOpts)
		return "", fmt.Errorf("failed to start container %s: %w", containerName, err)
	}

	return resp.ID, nil
}

// RemoveContainer stops and removes an AirPlay container
func (m *AirPlayManager) RemoveContainer(ctx context.Context, streamID int) error {
	containerName := fmt.Sprintf("airplay-%d", streamID)

	// Stop the container
	timeout := 10
	stopOpts := client.ContainerStopOptions{
		Timeout: &timeout,
	}
	if _, err := m.cli.ContainerStop(ctx, containerName, stopOpts); err != nil {
		// Container might already be stopped, continue to remove
	}

	// Remove the container
	rmOpts := client.ContainerRemoveOptions{
		Force:         true,
		RemoveVolumes: false, // Keep the config volume
	}
	if _, err := m.cli.ContainerRemove(ctx, containerName, rmOpts); err != nil {
		return fmt.Errorf("failed to remove container %s: %w", containerName, err)
	}

	return nil
}

// UpdateContainer updates an existing container's configuration (name change)
// Since the name is passed via environment variable, we need to recreate the container
func (m *AirPlayManager) UpdateContainer(ctx context.Context, streamID int, newName string) error {
	containerName := fmt.Sprintf("airplay-%d", streamID)

	// Get current container to preserve ALSA device assignment
	inspectOpts := client.ContainerInspectOptions{}
	inspect, err := m.cli.ContainerInspect(ctx, containerName, inspectOpts)
	if err != nil {
		return fmt.Errorf("container %s not found: %w", containerName, err)
	}

	// Extract ALSA device from environment
	alsaDevice := ""
	for _, env := range inspect.Config.Env {
		if len(env) > 12 && env[:12] == "ALSA_DEVICE=" {
			alsaDevice = env[12:]
			break
		}
	}
	if alsaDevice == "" {
		// Fallback to calculating from stream ID
		alsaDevice = fmt.Sprintf("lb%dc", streamID%8)
	}

	// Remove old container
	if err := m.RemoveContainer(ctx, streamID); err != nil {
		return fmt.Errorf("failed to remove old container: %w", err)
	}

	// Create new container with updated name
	_, err = m.CreateContainer(ctx, streamID, newName, alsaDevice)
	return err
}

// ListContainers returns all AirPlay containers managed by this instance
func (m *AirPlayManager) ListContainers(ctx context.Context) ([]string, error) {
	listOpts := client.ContainerListOptions{
		All: true,
	}
	result, err := m.cli.ContainerList(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var airplayContainers []string
	for _, c := range result.Items {
		for _, name := range c.Names {
			if len(name) > 9 && name[1:9] == "airplay-" { // Names start with /
				airplayContainers = append(airplayContainers, c.ID)
				break
			}
		}
	}

	return airplayContainers, nil
}

// allocateIP allocates the next available IP address
func (m *AirPlayManager) allocateIP() string {
	// Parse the base IP (e.g., "10.100.10.98" -> increment last octet)
	ip := net.ParseIP(m.ipRangeStart)
	if ip == nil {
		return m.ipRangeStart // Return as-is if parsing fails
	}

	// Increment the last octet
	ip = ip.To4()
	if ip != nil && m.nextIPOffset > 0 {
		ip[3] += byte(m.nextIPOffset)
	}
	m.nextIPOffset++

	return ip.String()
}

// Close closes the Docker client connection
func (m *AirPlayManager) Close() error {
	if m.cli != nil {
		return m.cli.Close()
	}
	return nil
}
