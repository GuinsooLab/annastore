// Copyright (c) 2015-2021 MinIO, Inc.
//
// This file is part of MinIO Object Storage stack
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/minio/madmin-go"
	"github.com/minio/minio/internal/config"
	"github.com/minio/minio/internal/config/api"
	"github.com/minio/minio/internal/config/cache"
	"github.com/minio/minio/internal/config/callhome"
	"github.com/minio/minio/internal/config/compress"
	"github.com/minio/minio/internal/config/dns"
	"github.com/minio/minio/internal/config/etcd"
	"github.com/minio/minio/internal/config/heal"
	xldap "github.com/minio/minio/internal/config/identity/ldap"
	"github.com/minio/minio/internal/config/identity/openid"
	idplugin "github.com/minio/minio/internal/config/identity/plugin"
	xtls "github.com/minio/minio/internal/config/identity/tls"
	"github.com/minio/minio/internal/config/notify"
	"github.com/minio/minio/internal/config/policy/opa"
	polplugin "github.com/minio/minio/internal/config/policy/plugin"
	"github.com/minio/minio/internal/config/scanner"
	"github.com/minio/minio/internal/config/storageclass"
	"github.com/minio/minio/internal/config/subnet"
	"github.com/minio/minio/internal/crypto"
	xhttp "github.com/minio/minio/internal/http"
	"github.com/minio/minio/internal/kms"
	"github.com/minio/minio/internal/logger"
	"github.com/minio/pkg/env"
)

func initHelp() {
	kvs := map[string]config.KVS{
		config.EtcdSubSys:           etcd.DefaultKVS,
		config.CacheSubSys:          cache.DefaultKVS,
		config.CompressionSubSys:    compress.DefaultKVS,
		config.IdentityLDAPSubSys:   xldap.DefaultKVS,
		config.IdentityOpenIDSubSys: openid.DefaultKVS,
		config.IdentityTLSSubSys:    xtls.DefaultKVS,
		config.IdentityPluginSubSys: idplugin.DefaultKVS,
		config.PolicyOPASubSys:      opa.DefaultKVS,
		config.PolicyPluginSubSys:   polplugin.DefaultKVS,
		config.SiteSubSys:           config.DefaultSiteKVS,
		config.RegionSubSys:         config.DefaultRegionKVS,
		config.APISubSys:            api.DefaultKVS,
		config.CredentialsSubSys:    config.DefaultCredentialKVS,
		config.LoggerWebhookSubSys:  logger.DefaultLoggerWebhookKVS,
		config.AuditWebhookSubSys:   logger.DefaultAuditWebhookKVS,
		config.AuditKafkaSubSys:     logger.DefaultAuditKafkaKVS,
		config.HealSubSys:           heal.DefaultKVS,
		config.ScannerSubSys:        scanner.DefaultKVS,
		config.SubnetSubSys:         subnet.DefaultKVS,
		config.CallhomeSubSys:       callhome.DefaultKVS,
	}
	for k, v := range notify.DefaultNotificationKVS {
		kvs[k] = v
	}
	if globalIsErasure {
		kvs[config.StorageClassSubSys] = storageclass.DefaultKVS
	}
	config.RegisterDefaultKVS(kvs)

	// Captures help for each sub-system
	helpSubSys := config.HelpKVS{
		config.HelpKV{
			Key:         config.SiteSubSys,
			Description: "label the server and its location",
		},
		config.HelpKV{
			Key:         config.CacheSubSys,
			Description: "add caching storage tier",
		},
		config.HelpKV{
			Key:         config.CompressionSubSys,
			Description: "enable server side compression of objects",
		},
		config.HelpKV{
			Key:         config.EtcdSubSys,
			Description: "federate multiple clusters for IAM and Bucket DNS",
		},
		config.HelpKV{
			Key:             config.IdentityOpenIDSubSys,
			Description:     "enable OpenID SSO support",
			MultipleTargets: true,
		},
		config.HelpKV{
			Key:         config.IdentityLDAPSubSys,
			Description: "enable LDAP SSO support",
		},
		config.HelpKV{
			Key:         config.IdentityTLSSubSys,
			Description: "enable X.509 TLS certificate SSO support",
		},
		config.HelpKV{
			Key:         config.IdentityPluginSubSys,
			Description: "enable Identity Plugin via external hook",
		},
		config.HelpKV{
			Key:         config.PolicyPluginSubSys,
			Description: "enable Access Management Plugin for policy enforcement",
		},
		config.HelpKV{
			Key:         config.APISubSys,
			Description: "manage global HTTP API call specific features, such as throttling, authentication types, etc.",
		},
		config.HelpKV{
			Key:         config.HealSubSys,
			Description: "manage object healing frequency and bitrot verification checks",
		},
		config.HelpKV{
			Key:         config.ScannerSubSys,
			Description: "manage namespace scanning for usage calculation, lifecycle, healing and more",
		},
		config.HelpKV{
			Key:             config.LoggerWebhookSubSys,
			Description:     "send server logs to webhook endpoints",
			MultipleTargets: true,
		},
		config.HelpKV{
			Key:             config.AuditWebhookSubSys,
			Description:     "send audit logs to webhook endpoints",
			MultipleTargets: true,
		},
		config.HelpKV{
			Key:             config.AuditKafkaSubSys,
			Description:     "send audit logs to kafka endpoints",
			MultipleTargets: true,
		},
		config.HelpKV{
			Key:             config.NotifyWebhookSubSys,
			Description:     "publish bucket notifications to webhook endpoints",
			MultipleTargets: true,
		},
		config.HelpKV{
			Key:             config.NotifyAMQPSubSys,
			Description:     "publish bucket notifications to AMQP endpoints",
			MultipleTargets: true,
		},
		config.HelpKV{
			Key:             config.NotifyKafkaSubSys,
			Description:     "publish bucket notifications to Kafka endpoints",
			MultipleTargets: true,
		},
		config.HelpKV{
			Key:             config.NotifyMQTTSubSys,
			Description:     "publish bucket notifications to MQTT endpoints",
			MultipleTargets: true,
		},
		config.HelpKV{
			Key:             config.NotifyNATSSubSys,
			Description:     "publish bucket notifications to NATS endpoints",
			MultipleTargets: true,
		},
		config.HelpKV{
			Key:             config.NotifyNSQSubSys,
			Description:     "publish bucket notifications to NSQ endpoints",
			MultipleTargets: true,
		},
		config.HelpKV{
			Key:             config.NotifyMySQLSubSys,
			Description:     "publish bucket notifications to MySQL databases",
			MultipleTargets: true,
		},
		config.HelpKV{
			Key:             config.NotifyPostgresSubSys,
			Description:     "publish bucket notifications to Postgres databases",
			MultipleTargets: true,
		},
		config.HelpKV{
			Key:             config.NotifyESSubSys,
			Description:     "publish bucket notifications to Elasticsearch endpoints",
			MultipleTargets: true,
		},
		config.HelpKV{
			Key:             config.NotifyRedisSubSys,
			Description:     "publish bucket notifications to Redis datastores",
			MultipleTargets: true,
		},
		config.HelpKV{
			Key:         config.SubnetSubSys,
			Type:        "string",
			Description: "set subnet config for the cluster e.g. api key",
			Optional:    true,
		},
		config.HelpKV{
			Key:         config.CallhomeSubSys,
			Type:        "string",
			Description: "enable callhome for the cluster",
			Optional:    true,
		},
	}

	if globalIsErasure {
		helpSubSys = append(helpSubSys, config.HelpKV{})
		copy(helpSubSys[2:], helpSubSys[1:])
		helpSubSys[1] = config.HelpKV{
			Key:         config.StorageClassSubSys,
			Description: "define object level redundancy",
		}
	}

	helpMap := map[string]config.HelpKVS{
		"":                          helpSubSys, // Help for all sub-systems.
		config.SiteSubSys:           config.SiteHelp,
		config.RegionSubSys:         config.RegionHelp,
		config.APISubSys:            api.Help,
		config.StorageClassSubSys:   storageclass.Help,
		config.EtcdSubSys:           etcd.Help,
		config.CacheSubSys:          cache.Help,
		config.CompressionSubSys:    compress.Help,
		config.HealSubSys:           heal.Help,
		config.ScannerSubSys:        scanner.Help,
		config.IdentityOpenIDSubSys: openid.Help,
		config.IdentityLDAPSubSys:   xldap.Help,
		config.IdentityTLSSubSys:    xtls.Help,
		config.IdentityPluginSubSys: idplugin.Help,
		config.PolicyOPASubSys:      opa.Help,
		config.PolicyPluginSubSys:   polplugin.Help,
		config.LoggerWebhookSubSys:  logger.Help,
		config.AuditWebhookSubSys:   logger.HelpWebhook,
		config.AuditKafkaSubSys:     logger.HelpKafka,
		config.NotifyAMQPSubSys:     notify.HelpAMQP,
		config.NotifyKafkaSubSys:    notify.HelpKafka,
		config.NotifyMQTTSubSys:     notify.HelpMQTT,
		config.NotifyNATSSubSys:     notify.HelpNATS,
		config.NotifyNSQSubSys:      notify.HelpNSQ,
		config.NotifyMySQLSubSys:    notify.HelpMySQL,
		config.NotifyPostgresSubSys: notify.HelpPostgres,
		config.NotifyRedisSubSys:    notify.HelpRedis,
		config.NotifyWebhookSubSys:  notify.HelpWebhook,
		config.NotifyESSubSys:       notify.HelpES,
		config.SubnetSubSys:         subnet.HelpSubnet,
		config.CallhomeSubSys:       callhome.HelpCallhome,
	}

	config.RegisterHelpSubSys(helpMap)

	// save top-level help for deprecated sub-systems in a separate map.
	deprecatedHelpKVMap := map[string]config.HelpKV{
		config.RegionSubSys: {
			Key:         config.RegionSubSys,
			Description: "[DEPRECATED - use `site` instead] label the location of the server",
		},
		config.PolicyOPASubSys: {
			Key:         config.PolicyOPASubSys,
			Description: "[DEPRECATED - use `policy_plugin` instead] enable external OPA for policy enforcement",
		},
	}

	config.RegisterHelpDeprecatedSubSys(deprecatedHelpKVMap)
}

var (
	// globalServerConfig server config.
	globalServerConfig   config.Config
	globalServerConfigMu sync.RWMutex
)

func validateSubSysConfig(s config.Config, subSys string, objAPI ObjectLayer) error {
	switch subSys {
	case config.CredentialsSubSys:
		if _, err := config.LookupCreds(s[config.CredentialsSubSys][config.Default]); err != nil {
			return err
		}
	case config.SiteSubSys:
		if _, err := config.LookupSite(s[config.SiteSubSys][config.Default], s[config.RegionSubSys][config.Default]); err != nil {
			return err
		}
	case config.APISubSys:
		if _, err := api.LookupConfig(s[config.APISubSys][config.Default]); err != nil {
			return err
		}
	case config.StorageClassSubSys:
		if globalIsErasure {
			if objAPI == nil {
				return errServerNotInitialized
			}
			for _, setDriveCount := range objAPI.SetDriveCounts() {
				if _, err := storageclass.LookupConfig(s[config.StorageClassSubSys][config.Default], setDriveCount); err != nil {
					return err
				}
			}
		}
	case config.CacheSubSys:
		if _, err := cache.LookupConfig(s[config.CacheSubSys][config.Default]); err != nil {
			return err
		}
	case config.CompressionSubSys:
		compCfg, err := compress.LookupConfig(s[config.CompressionSubSys][config.Default])
		if err != nil {
			return err
		}

		if objAPI != nil {
			if compCfg.Enabled && !objAPI.IsCompressionSupported() {
				return fmt.Errorf("Backend does not support compression")
			}
		}
	case config.HealSubSys:
		if _, err := heal.LookupConfig(s[config.HealSubSys][config.Default]); err != nil {
			return err
		}
	case config.ScannerSubSys:
		if _, err := scanner.LookupConfig(s[config.ScannerSubSys][config.Default]); err != nil {
			return err
		}
	case config.EtcdSubSys:
		etcdCfg, err := etcd.LookupConfig(s[config.EtcdSubSys][config.Default], globalRootCAs)
		if err != nil {
			return err
		}
		if etcdCfg.Enabled {
			etcdClnt, err := etcd.New(etcdCfg)
			if err != nil {
				return err
			}
			etcdClnt.Close()
		}
	case config.IdentityOpenIDSubSys:
		if _, err := openid.LookupConfig(s,
			NewGatewayHTTPTransport(), xhttp.DrainBody, globalSite.Region); err != nil {
			return err
		}
	case config.IdentityLDAPSubSys:
		cfg, err := xldap.Lookup(s[config.IdentityLDAPSubSys][config.Default], globalRootCAs)
		if err != nil {
			return err
		}
		if cfg.Enabled {
			conn, cerr := cfg.Connect()
			if cerr != nil {
				return cerr
			}
			conn.Close()
		}
	case config.IdentityTLSSubSys:
		if _, err := xtls.Lookup(s[config.IdentityTLSSubSys][config.Default]); err != nil {
			return err
		}
	case config.IdentityPluginSubSys:
		if _, err := idplugin.LookupConfig(s[config.IdentityPluginSubSys][config.Default],
			NewGatewayHTTPTransport(), xhttp.DrainBody, globalSite.Region); err != nil {
			return err
		}
	case config.SubnetSubSys:
		if _, err := subnet.LookupConfig(s[config.SubnetSubSys][config.Default], nil); err != nil {
			return err
		}
	case config.CallhomeSubSys:
		if _, err := callhome.LookupConfig(s[config.CallhomeSubSys][config.Default]); err != nil {
			return err
		}
	case config.PolicyOPASubSys:
		// In case legacy OPA config is being set, we treat it as if the
		// AuthZPlugin is being set.
		subSys = config.PolicyPluginSubSys
		fallthrough
	case config.PolicyPluginSubSys:
		if ppargs, err := polplugin.LookupConfig(s[config.PolicyPluginSubSys][config.Default],
			NewGatewayHTTPTransport(), xhttp.DrainBody); err != nil {
			return err
		} else if ppargs.URL == nil {
			// Check if legacy opa is configured.
			if _, err := opa.LookupConfig(s[config.PolicyOPASubSys][config.Default],
				NewGatewayHTTPTransport(), xhttp.DrainBody); err != nil {
				return err
			}
		}
	default:
		if config.LoggerSubSystems.Contains(subSys) {
			if err := logger.ValidateSubSysConfig(s, subSys); err != nil {
				return err
			}
		}
	}

	if config.NotifySubSystems.Contains(subSys) {
		if err := notify.TestSubSysNotificationTargets(GlobalContext, s, NewGatewayHTTPTransport(), globalNotificationSys.ConfiguredTargetIDs(), subSys); err != nil {
			return err
		}
	}
	return nil
}

func validateConfig(s config.Config, subSys string) error {
	objAPI := newObjectLayerFn()

	// We must have a global lock for this so nobody else modifies env while we do.
	defer env.LockSetEnv()()

	// Disable merging env values with config for validation.
	env.SetEnvOff()

	// Enable env values to validate KMS.
	defer env.SetEnvOn()
	if subSys != "" {
		return validateSubSysConfig(s, subSys, objAPI)
	}

	// No sub-system passed. Validate all of them.
	for _, ss := range config.SubSystems.ToSlice() {
		if err := validateSubSysConfig(s, ss, objAPI); err != nil {
			return err
		}
	}

	return nil
}

func lookupConfigs(s config.Config, objAPI ObjectLayer) {
	ctx := GlobalContext

	var err error
	if !globalActiveCred.IsValid() {
		// Env doesn't seem to be set, we fallback to lookup creds from the config.
		globalActiveCred, err = config.LookupCreds(s[config.CredentialsSubSys][config.Default])
		if err != nil {
			logger.LogIf(ctx, fmt.Errorf("Invalid credentials configuration: %w", err))
		}
	}

	dnsURL, dnsUser, dnsPass, err := env.LookupEnv(config.EnvDNSWebhook)
	if err != nil {
		if globalIsGateway {
			logger.FatalIf(err, "Unable to initialize remote webhook DNS config")
		} else {
			logger.LogIf(ctx, fmt.Errorf("Unable to initialize remote webhook DNS config %w", err))
		}
	}
	if err == nil && dnsURL != "" {
		globalDNSConfig, err = dns.NewOperatorDNS(dnsURL,
			dns.Authentication(dnsUser, dnsPass),
			dns.RootCAs(globalRootCAs))
		if err != nil {
			if globalIsGateway {
				logger.FatalIf(err, "Unable to initialize remote webhook DNS config")
			} else {
				logger.LogIf(ctx, fmt.Errorf("Unable to initialize remote webhook DNS config %w", err))
			}
		}
	}

	etcdCfg, err := etcd.LookupConfig(s[config.EtcdSubSys][config.Default], globalRootCAs)
	if err != nil {
		if globalIsGateway {
			logger.FatalIf(err, "Unable to initialize etcd config")
		} else {
			logger.LogIf(ctx, fmt.Errorf("Unable to initialize etcd config: %w", err))
		}
	}

	if etcdCfg.Enabled {
		if globalEtcdClient == nil {
			globalEtcdClient, err = etcd.New(etcdCfg)
			if err != nil {
				if globalIsGateway {
					logger.FatalIf(err, "Unable to initialize etcd config")
				} else {
					logger.LogIf(ctx, fmt.Errorf("Unable to initialize etcd config: %w", err))
				}
			}
		}

		if len(globalDomainNames) != 0 && !globalDomainIPs.IsEmpty() && globalEtcdClient != nil {
			if globalDNSConfig != nil {
				// if global DNS is already configured, indicate with a warning, incase
				// users are confused.
				logger.LogIf(ctx, fmt.Errorf("DNS store is already configured with %s, not using etcd for DNS store", globalDNSConfig))
			} else {
				globalDNSConfig, err = dns.NewCoreDNS(etcdCfg.Config,
					dns.DomainNames(globalDomainNames),
					dns.DomainIPs(globalDomainIPs),
					dns.DomainPort(globalMinioPort),
					dns.CoreDNSPath(etcdCfg.CoreDNSPath),
				)
				if err != nil {
					if globalIsGateway {
						logger.FatalIf(err, "Unable to initialize DNS config")
					} else {
						logger.LogIf(ctx, fmt.Errorf("Unable to initialize DNS config for %s: %w",
							globalDomainNames, err))
					}
				}
			}
		}
	}

	// Bucket federation is 'true' only when IAM assets are not namespaced
	// per tenant and all tenants interested in globally available users
	// if namespace was requested such as specifying etcdPathPrefix then
	// we assume that users are interested in global bucket support
	// but not federation.
	globalBucketFederation = etcdCfg.PathPrefix == "" && etcdCfg.Enabled

	globalSite, err = config.LookupSite(s[config.SiteSubSys][config.Default], s[config.RegionSubSys][config.Default])
	if err != nil {
		logger.LogIf(ctx, fmt.Errorf("Invalid site configuration: %w", err))
	}

	apiConfig, err := api.LookupConfig(s[config.APISubSys][config.Default])
	if err != nil {
		logger.LogIf(ctx, fmt.Errorf("Invalid api configuration: %w", err))
	}

	// Initialize remote instance transport once.
	getRemoteInstanceTransportOnce.Do(func() {
		getRemoteInstanceTransport = newGatewayHTTPTransport(apiConfig.RemoteTransportDeadline)
	})

	globalCacheConfig, err = cache.LookupConfig(s[config.CacheSubSys][config.Default])
	if err != nil {
		if globalIsGateway {
			logger.FatalIf(err, "Unable to setup cache")
		} else {
			logger.LogIf(ctx, fmt.Errorf("Unable to setup cache: %w", err))
		}
	}

	if globalCacheConfig.Enabled {
		if cacheEncKey := env.Get(cache.EnvCacheEncryptionKey, ""); cacheEncKey != "" {
			globalCacheKMS, err = kms.Parse(cacheEncKey)
			if err != nil {
				logger.LogIf(ctx, fmt.Errorf("Unable to setup encryption cache: %w", err))
			}
		}
	}

	globalAutoEncryption = crypto.LookupAutoEncryption() // Enable auto-encryption if enabled
	if globalAutoEncryption && GlobalKMS == nil {
		logger.Fatal(errors.New("no KMS configured"), "MINIO_KMS_AUTO_ENCRYPTION requires a valid KMS configuration")
	}

	globalSTSTLSConfig, err = xtls.Lookup(s[config.IdentityTLSSubSys][config.Default])
	if err != nil {
		logger.LogIf(ctx, fmt.Errorf("Unable to initialize X.509/TLS STS API: %w", err))
	}

	if globalSTSTLSConfig.InsecureSkipVerify {
		logger.LogIf(ctx, fmt.Errorf("CRITICAL: enabling %s is not recommended in a production environment", xtls.EnvIdentityTLSSkipVerify))
	}

	globalOpenIDConfig, err = openid.LookupConfig(s,
		NewGatewayHTTPTransport(), xhttp.DrainBody, globalSite.Region)
	if err != nil {
		logger.LogIf(ctx, fmt.Errorf("Unable to initialize OpenID: %w", err))
	}

	globalLDAPConfig, err = xldap.Lookup(s[config.IdentityLDAPSubSys][config.Default],
		globalRootCAs)
	if err != nil {
		logger.LogIf(ctx, fmt.Errorf("Unable to parse LDAP configuration: %w", err))
	}

	authNPluginCfg, err := idplugin.LookupConfig(s[config.IdentityPluginSubSys][config.Default],
		NewGatewayHTTPTransport(), xhttp.DrainBody, globalSite.Region)
	if err != nil {
		logger.LogIf(ctx, fmt.Errorf("Unable to initialize AuthNPlugin: %w", err))
	}
	globalAuthNPlugin = idplugin.New(authNPluginCfg)

	authZPluginCfg, err := polplugin.LookupConfig(s[config.PolicyPluginSubSys][config.Default],
		NewGatewayHTTPTransport(), xhttp.DrainBody)
	if err != nil {
		logger.LogIf(ctx, fmt.Errorf("Unable to initialize AuthZPlugin: %w", err))
	}
	if authZPluginCfg.URL == nil {
		opaCfg, err := opa.LookupConfig(s[config.PolicyOPASubSys][config.Default],
			NewGatewayHTTPTransport(), xhttp.DrainBody)
		if err != nil {
			logger.LogIf(ctx, fmt.Errorf("Unable to initialize AuthZPlugin from legacy OPA config: %w", err))
		} else {
			authZPluginCfg.URL = opaCfg.URL
			authZPluginCfg.AuthToken = opaCfg.AuthToken
			authZPluginCfg.Transport = opaCfg.Transport
			authZPluginCfg.CloseRespFn = opaCfg.CloseRespFn
		}
	}

	setGlobalAuthZPlugin(polplugin.New(authZPluginCfg))

	globalSubnetConfig, err = subnet.LookupConfig(s[config.SubnetSubSys][config.Default], globalProxyTransport)
	if err != nil {
		logger.LogIf(ctx, fmt.Errorf("Unable to parse subnet configuration: %w", err))
	}

	globalConfigTargetList, err = notify.GetNotificationTargets(GlobalContext, s, NewGatewayHTTPTransport(), false)
	if err != nil {
		logger.LogIf(ctx, fmt.Errorf("Unable to initialize notification target(s): %w", err))
	}

	globalEnvTargetList, err = notify.GetNotificationTargets(GlobalContext, newServerConfig(), NewGatewayHTTPTransport(), true)
	if err != nil {
		logger.LogIf(ctx, fmt.Errorf("Unable to initialize notification target(s): %w", err))
	}

	// Apply dynamic config values
	if err := applyDynamicConfig(ctx, objAPI, s); err != nil {
		if globalIsGateway {
			logger.FatalIf(err, "Unable to initialize dynamic configuration")
		} else {
			logger.LogIf(ctx, err)
		}
	}
}

func applyDynamicConfigForSubSys(ctx context.Context, objAPI ObjectLayer, s config.Config, subSys string) error {
	switch subSys {
	case config.APISubSys:
		apiConfig, err := api.LookupConfig(s[config.APISubSys][config.Default])
		if err != nil {
			logger.LogIf(ctx, fmt.Errorf("Invalid api configuration: %w", err))
		}
		var setDriveCounts []int
		if objAPI != nil {
			setDriveCounts = objAPI.SetDriveCounts()
		}
		globalAPIConfig.init(apiConfig, setDriveCounts)
	case config.CompressionSubSys:
		cmpCfg, err := compress.LookupConfig(s[config.CompressionSubSys][config.Default])
		if err != nil {
			return fmt.Errorf("Unable to setup Compression: %w", err)
		}
		// Validate if the object layer supports compression.
		if objAPI != nil {
			if cmpCfg.Enabled && !objAPI.IsCompressionSupported() {
				return fmt.Errorf("Backend does not support compression")
			}
		}
		globalCompressConfigMu.Lock()
		globalCompressConfig = cmpCfg
		globalCompressConfigMu.Unlock()
	case config.HealSubSys:
		healCfg, err := heal.LookupConfig(s[config.HealSubSys][config.Default])
		if err != nil {
			return fmt.Errorf("Unable to apply heal config: %w", err)
		}
		globalHealConfig.Update(healCfg)
	case config.ScannerSubSys:
		scannerCfg, err := scanner.LookupConfig(s[config.ScannerSubSys][config.Default])
		if err != nil {
			return fmt.Errorf("Unable to apply scanner config: %w", err)
		}
		// update dynamic scanner values.
		scannerCycle.Store(scannerCfg.Cycle)
		logger.LogIf(ctx, scannerSleeper.Update(scannerCfg.Delay, scannerCfg.MaxWait))
	case config.LoggerWebhookSubSys:
		loggerCfg, err := logger.LookupConfigForSubSys(s, config.LoggerWebhookSubSys)
		if err != nil {
			logger.LogIf(ctx, fmt.Errorf("Unable to load logger webhook config: %w", err))
		}
		userAgent := getUserAgent(getMinioMode())
		for n, l := range loggerCfg.HTTP {
			if l.Enabled {
				l.LogOnce = logger.LogOnceConsoleIf
				l.UserAgent = userAgent
				l.Transport = NewGatewayHTTPTransportWithClientCerts(l.ClientCert, l.ClientKey)
				loggerCfg.HTTP[n] = l
			}
		}
		err = logger.UpdateSystemTargets(loggerCfg)
		if err != nil {
			logger.LogIf(ctx, fmt.Errorf("Unable to update logger webhook config: %w", err))
		}
	case config.AuditWebhookSubSys:
		loggerCfg, err := logger.LookupConfigForSubSys(s, config.AuditWebhookSubSys)
		if err != nil {
			logger.LogIf(ctx, fmt.Errorf("Unable to load audit webhook config: %w", err))
		}
		userAgent := getUserAgent(getMinioMode())
		for n, l := range loggerCfg.AuditWebhook {
			if l.Enabled {
				l.LogOnce = logger.LogOnceConsoleIf
				l.UserAgent = userAgent
				l.Transport = NewGatewayHTTPTransportWithClientCerts(l.ClientCert, l.ClientKey)
				loggerCfg.AuditWebhook[n] = l
			}
		}

		err = logger.UpdateAuditWebhookTargets(loggerCfg)
		if err != nil {
			logger.LogIf(ctx, fmt.Errorf("Unable to update audit webhook targets: %w", err))
		}
	case config.AuditKafkaSubSys:
		loggerCfg, err := logger.LookupConfigForSubSys(s, config.AuditKafkaSubSys)
		if err != nil {
			logger.LogIf(ctx, fmt.Errorf("Unable to load audit kafka config: %w", err))
		}
		for n, l := range loggerCfg.AuditKafka {
			if l.Enabled {
				l.LogOnce = logger.LogOnceIf
				loggerCfg.AuditKafka[n] = l
			}
		}
		err = logger.UpdateAuditKafkaTargets(loggerCfg)
		if err != nil {
			logger.LogIf(ctx, fmt.Errorf("Unable to update audit kafka targets: %w", err))
		}
	case config.StorageClassSubSys:
		if globalIsErasure && objAPI != nil {
			setDriveCounts := objAPI.SetDriveCounts()
			for i, setDriveCount := range setDriveCounts {
				sc, err := storageclass.LookupConfig(s[config.StorageClassSubSys][config.Default], setDriveCount)
				if err != nil {
					logger.LogIf(ctx, fmt.Errorf("Unable to initialize storage class config: %w", err))
					break
				}
				// if we validated all setDriveCounts and it was successful
				// proceed to store the correct storage class globally.
				if i == len(setDriveCounts)-1 {
					globalStorageClass.Update(sc)
				}
			}
		}
	case config.CallhomeSubSys:
		callhomeCfg, err := callhome.LookupConfig(s[config.CallhomeSubSys][config.Default])
		if err != nil {
			logger.LogIf(ctx, fmt.Errorf("Unable to load callhome config: %w", err))
		} else {
			globalCallhomeConfig = callhomeCfg
			updateCallhomeParams(ctx, objAPI)
		}
	}
	globalServerConfigMu.Lock()
	defer globalServerConfigMu.Unlock()
	if globalServerConfig != nil {
		globalServerConfig[subSys] = s[subSys]
	}
	return nil
}

// applyDynamicConfig will apply dynamic config values.
// Dynamic systems should be in config.SubSystemsDynamic as well.
func applyDynamicConfig(ctx context.Context, objAPI ObjectLayer, s config.Config) error {
	for subSys := range config.SubSystemsDynamic {
		err := applyDynamicConfigForSubSys(ctx, objAPI, s, subSys)
		if err != nil {
			return err
		}
	}
	return nil
}

// Help - return sub-system level help
type Help struct {
	SubSys          string         `json:"subSys"`
	Description     string         `json:"description"`
	MultipleTargets bool           `json:"multipleTargets"`
	KeysHelp        config.HelpKVS `json:"keysHelp"`
}

// GetHelp - returns help for sub-sys, a key for a sub-system or all the help.
func GetHelp(subSys, key string, envOnly bool) (Help, error) {
	if len(subSys) == 0 {
		return Help{KeysHelp: config.HelpSubSysMap[subSys]}, nil
	}
	subSystemValue := strings.SplitN(subSys, config.SubSystemSeparator, 2)
	if len(subSystemValue) == 0 {
		return Help{}, config.Errorf("invalid number of arguments %s", subSys)
	}

	subSys = subSystemValue[0]

	subSysHelp, ok := config.HelpSubSysMap[""].Lookup(subSys)
	if !ok {
		subSysHelp, ok = config.HelpDeprecatedSubSysMap[subSys]
		if !ok {
			return Help{}, config.Errorf("unknown sub-system %s", subSys)
		}
	}

	h, ok := config.HelpSubSysMap[subSys]
	if !ok {
		return Help{}, config.Errorf("unknown sub-system %s", subSys)
	}
	if key != "" {
		value, ok := h.Lookup(key)
		if !ok {
			return Help{}, config.Errorf("unknown key %s for sub-system %s",
				key, subSys)
		}
		h = config.HelpKVS{value}
	}

	envHelp := config.HelpKVS{}
	if envOnly {
		// Only for multiple targets, make sure
		// to list the ENV, for regular k/v EnableKey is
		// implicit, for ENVs we cannot make it implicit.
		if subSysHelp.MultipleTargets {
			envK := config.EnvPrefix + strings.ToTitle(subSys) + config.EnvWordDelimiter + strings.ToTitle(madmin.EnableKey)
			envHelp = append(envHelp, config.HelpKV{
				Key:         envK,
				Description: fmt.Sprintf("enable %s target, default is 'off'", subSys),
				Optional:    false,
				Type:        "on|off",
			})
		}
		for _, hkv := range h {
			envK := config.EnvPrefix + strings.ToTitle(subSys) + config.EnvWordDelimiter + strings.ToTitle(hkv.Key)
			envHelp = append(envHelp, config.HelpKV{
				Key:         envK,
				Description: hkv.Description,
				Optional:    hkv.Optional,
				Type:        hkv.Type,
			})
		}
		h = envHelp
	}

	return Help{
		SubSys:          subSys,
		Description:     subSysHelp.Description,
		MultipleTargets: subSysHelp.MultipleTargets,
		KeysHelp:        h,
	}, nil
}

func newServerConfig() config.Config {
	return config.New()
}

// newSrvConfig - initialize a new server config, saves env parameters if
// found, otherwise use default parameters
func newSrvConfig(objAPI ObjectLayer) error {
	// Initialize server config.
	srvCfg := newServerConfig()

	// hold the mutex lock before a new config is assigned.
	globalServerConfigMu.Lock()
	globalServerConfig = srvCfg
	globalServerConfigMu.Unlock()

	// Save config into file.
	return saveServerConfig(GlobalContext, objAPI, globalServerConfig)
}

func getValidConfig(objAPI ObjectLayer) (config.Config, error) {
	return readServerConfig(GlobalContext, objAPI)
}

// loadConfig - loads a new config from disk, overrides params
// from env if found and valid
func loadConfig(objAPI ObjectLayer) error {
	srvCfg, err := getValidConfig(objAPI)
	if err != nil {
		return err
	}

	// Override any values from ENVs.
	lookupConfigs(srvCfg, objAPI)

	// hold the mutex lock before a new config is assigned.
	globalServerConfigMu.Lock()
	globalServerConfig = srvCfg
	globalServerConfigMu.Unlock()

	return nil
}
