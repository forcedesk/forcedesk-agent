<?php

namespace App\Console\Commands;

use App\Helpers\AgentConfig;
use App\Models\AgentSetting;
use Illuminate\Console\Command;

class ManageAgentSettings extends Command
{
    protected $signature = 'agent:settings
                            {action : The action to perform (import, export, list, get, set, clear-cache)}
                            {key? : The setting key (for get/set actions)}
                            {value? : The value to set (for set action)}';

    protected $description = 'Manage agent configuration settings';

    public function handle()
    {
        $action = $this->argument('action');

        return match ($action) {
            'import' => $this->importSettings(),
            'export' => $this->exportSettings(),
            'list' => $this->listSettings(),
            'get' => $this->getSetting(),
            'set' => $this->setSetting(),
            'clear-cache' => $this->clearCache(),
            default => $this->error("Unknown action: {$action}"),
        };
    }

    protected function importSettings()
    {
        $count = AgentConfig::importFromConfig();
        $this->info("Successfully imported {$count} settings from config file.");
        return 0;
    }

    protected function exportSettings()
    {
        $this->warn('Export functionality not yet implemented.');
        $this->info('Settings are automatically read from database with config file fallback.');
        return 0;
    }

    protected function listSettings()
    {
        $settings = AgentSetting::all()->groupBy('group');

        foreach ($settings as $group => $groupSettings) {
            $this->info("\n{$group}:");
            foreach ($groupSettings as $setting) {
                $value = $setting->is_sensitive ? '********' : $setting->value;
                $this->line("  {$setting->key} = {$value}");
            }
        }

        return 0;
    }

    protected function getSetting()
    {
        $key = $this->argument('key');

        if (!$key) {
            $this->error('Key argument is required for get action');
            return 1;
        }

        $value = agent_config($key);
        $this->info("{$key} = {$value}");

        return 0;
    }

    protected function setSetting()
    {
        $key = $this->argument('key');
        $value = $this->argument('value');

        if (!$key || $value === null) {
            $this->error('Both key and value arguments are required for set action');
            return 1;
        }

        // Detect if this is a sensitive key
        $isSensitive = str_contains(strtolower($key), 'password') ||
                      str_contains(strtolower($key), 'api_key') ||
                      str_contains(strtolower($key), 'secret');

        AgentConfig::set($key, $value, 'string', $isSensitive);
        $this->info("Successfully set {$key}");

        return 0;
    }

    protected function clearCache()
    {
        AgentConfig::clearCache();
        $this->info('Settings cache cleared successfully.');

        return 0;
    }
}
