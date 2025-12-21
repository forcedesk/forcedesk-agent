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

                // Only import if not already in database
                if (AgentSetting::where('key', $fullKey)->doesntExist()) {
                    self::set($fullKey, $value, $type, $isSensitive);
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
}
