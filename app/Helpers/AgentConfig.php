<?php

namespace App\Helpers;

use App\Models\AgentSetting;
use Illuminate\Support\Facades\Config;

class AgentConfig
{
    /**
     * Get a configuration value from database or fallback to config file
     *
     * @param string $key The config key (e.g., 'tenant.tenant_url')
     * @param mixed $default Default value if not found
     * @return mixed
     */
    public static function get(string $key, mixed $default = null): mixed
    {
        // Try to get from database first
        $dbValue = AgentSetting::getValue($key, null);

        if ($dbValue !== null) {
            return $dbValue;
        }

        // Fallback to config file
        $configKey = "agentconfig.{$key}";
        $configValue = Config::get($configKey, null);

        if ($configValue !== null) {
            return $configValue;
        }

        return $default;
    }

    /**
     * Set a configuration value in the database
     *
     * @param string $key The config key (e.g., 'tenant.tenant_url')
     * @param mixed $value The value to set
     * @param string $type The data type (string, boolean, integer, json)
     * @param bool $isSensitive Whether this is a sensitive value
     * @return void
     */
    public static function set(string $key, mixed $value, string $type = 'string', bool $isSensitive = false): void
    {
        // Extract group from key (e.g., 'tenant' from 'tenant.tenant_url')
        $parts = explode('.', $key);
        $group = $parts[0] ?? 'general';

        AgentSetting::setValue($key, $value, $type, $group, $isSensitive);
    }

    /**
     * Get all settings grouped by category
     *
     * @return array
     */
    public static function getAllGrouped(): array
    {
        return AgentSetting::getAllGrouped();
    }

    /**
     * Get all settings from a specific group
     *
     * @param string $group The group name (e.g., 'tenant', 'ldap')
     * @return array
     */
    public static function getGroup(string $group): array
    {
        $allGrouped = self::getAllGrouped();
        return $allGrouped[$group] ?? [];
    }

    /**
     * Import settings from config file to database
     *
     * @return int Number of settings imported
     */
    public static function importFromConfig(): int
    {
        $config = Config::get('agentconfig', []);
        $count = 0;

        foreach ($config as $group => $settings) {
            if (!is_array($settings)) {
                continue;
            }

            foreach ($settings as $key => $value) {
                $fullKey = "{$group}.{$key}";
                $type = self::detectType($value);
                $isSensitive = self::isSensitiveKey($key);
                $label = self::getFieldLabel($fullKey);
                $description = self::getFieldDescription($fullKey);

                // Only import if not already in database
                if (AgentSetting::where('key', $fullKey)->doesntExist()) {
                    AgentSetting::create([
                        'key' => $fullKey,
                        'value' => $value,
                        'type' => $type,
                        'group' => $group,
                        'is_sensitive' => $isSensitive,
                        'label' => $label,
                        'description' => $description,
                    ]);
                    $count++;
                }
            }
        }

        return $count;
    }

    /**
     * Detect the type of a value
     */
    protected static function detectType(mixed $value): string
    {
        if (is_bool($value)) {
            return 'boolean';
        }
        if (is_int($value)) {
            return 'integer';
        }
        if (is_float($value)) {
            return 'float';
        }
        if (is_array($value)) {
            return 'json';
        }
        return 'string';
    }

    /**
     * Check if a key is sensitive (password, api_key, etc.)
     */
    protected static function isSensitiveKey(string $key): bool
    {
        $sensitivePatterns = ['password', 'api_key', 'secret', 'token', 'key'];

        foreach ($sensitivePatterns as $pattern) {
            if (str_contains(strtolower($key), $pattern)) {
                return true;
            }
        }

        return false;
    }

    /**
     * Clear all setting caches
     */
    public static function clearCache(): void
    {
        AgentSetting::clearCache();
    }

    /**
     * Get a short label for a field
     */
    protected static function getFieldLabel(string $key): ?string
    {
        $labels = [
            // Tenant settings
            'tenant.tenant_url' => 'Tenant URL',
            'tenant.tenant_api_key' => 'API Key',
            'tenant.tenant_uuid' => 'Agent UUID',
            'tenant.verify_ssl' => 'Verify SSL',

            // Device Manager settings
            'device_manager.legacycommand' => 'Legacy Command',

            // Logging settings
            'logging.enabled' => 'Enable Logging',

            // PaperCut settings
            'papercut.api_url' => 'PaperCut Server URL',
            'papercut.api_key' => 'PaperCut API Key',
            'papercut.enabled' => 'Enable PaperCut',

            // Proxy settings
            'proxies.address' => 'Proxy Address',

            // EMC/Edustar settings
            'emc.emc_url' => 'EduSTAR MC URL',
            'emc.emc_username' => 'Username',
            'emc.emc_password' => 'Password',
            'emc.emc_school_code' => 'School Code',
            'emc.emc_crt_group_dn' => 'CRT Group DN',
            'emc.emc_crt_group_name' => 'CRT Group Name',

            // LDAP settings
            'ldap.ad_dc' => 'Domain Controller',
            'ldap.ad_svc_user_cn' => 'Service Account Username',
            'ldap.ad_svc_password' => 'Service Account Password',
            'ldap.ad_base_dn' => 'Base DN',
            'ldap.staff_scope' => 'Staff Group DN',
            'ldap.student_scope' => 'Student Group DN',
        ];

        return $labels[$key] ?? null;
    }

    /**
     * Get a descriptive label for a field
     */
    protected static function getFieldDescription(string $key): ?string
    {
        $descriptions = [
            // Tenant settings
            'tenant.tenant_url' => 'The URL of your ForceDesk tenant (e.g., https://your-tenant.forcedesk.io)',
            'tenant.tenant_api_key' => 'API key for authenticating with the ForceDesk tenant',
            'tenant.tenant_uuid' => 'Unique identifier for this agent instance',
            'tenant.verify_ssl' => 'Verify SSL certificates when connecting to the tenant',

            // Device Manager settings
            'device_manager.legacycommand' => 'Legacy SSH command options for older Cisco devices',

            // Logging settings
            'logging.enabled' => 'Enable or disable detailed logging for this agent',

            // PaperCut settings
            'papercut.api_url' => 'Your PaperCut server API URL (e.g., http://papercut-server:9191/api)',
            'papercut.api_key' => 'Authentication key for PaperCut API access',
            'papercut.enabled' => 'Enable or disable PaperCut integration',

            // Proxy settings
            'proxies.address' => 'HTTP/HTTPS proxy server address (e.g., http://proxy.example.com:8080)',

            // EMC/Edustar settings
            'emc.emc_url' => 'Edustar Management Console API endpoint URL',
            'emc.emc_username' => 'Username for Edustar MC authentication',
            'emc.emc_password' => 'Password for Edustar MC authentication',
            'emc.emc_school_code' => 'Your school code in the Edustar system',
            'emc.emc_crt_group_dn' => 'Distinguished Name of the CRT group in Active Directory',
            'emc.emc_crt_group_name' => 'Name of the CRT group for student accounts',

            // LDAP settings
            'ldap.ad_dc' => 'Primary Active Directory domain controller hostname or IP',
            'ldap.ad_svc_user_cn' => 'Service account username (CN) for LDAP operations',
            'ldap.ad_svc_password' => 'Service account password for LDAP authentication',
            'ldap.ad_base_dn' => 'Base Distinguished Name for LDAP searches (e.g., DC=school,DC=local)',
            'ldap.staff_scope' => 'LDAP group DN containing staff members',
            'ldap.student_scope' => 'LDAP group DN containing student accounts',
        ];

        return $descriptions[$key] ?? null;
    }
}
