<?php

namespace App\Http\Controllers;

use App\Helper\AgentConnectivityHelper;
use App\Helpers\AgentConfig;
use App\Models\AgentSetting;
use Illuminate\Http\Request;
use Inertia\Inertia;
use Inertia\Response;

class AgentSettingsController extends Controller
{
    /**
     * Display the settings management page
     */
    public function index(): Response
    {
        $settings = AgentSetting::all()->groupBy('group')->map(function ($groupSettings) {
            return $groupSettings->map(function ($setting) {
                return [
                    'id' => $setting->id,
                    'key' => $setting->key,
                    'value' => $setting->is_sensitive ? '********' : $setting->value,
                    'actual_value' => $setting->value,
                    'type' => $setting->type,
                    'label' => $setting->label,
                    'description' => $setting->description,
                    'is_sensitive' => $setting->is_sensitive,
                ];
            })->values();
        });

        return Inertia::render('AgentConfig/Index', [
            'settings' => $settings,
            'groups' => $settings->keys(),
        ]);
    }

    /**
     * Get all settings
     */
    public function getAll()
    {
        $settings = AgentSetting::all()->groupBy('group')->map(function ($groupSettings) {
            return $groupSettings->map(function ($setting) {
                return [
                    'id' => $setting->id,
                    'key' => $setting->key,
                    'value' => $setting->is_sensitive ? '********' : $setting->value,
                    'actual_value' => $setting->value,
                    'type' => $setting->type,
                    'label' => $setting->label,
                    'description' => $setting->description,
                    'is_sensitive' => $setting->is_sensitive,
                ];
            })->values();
        });

        return response()->json($settings);
    }

    /**
     * Update settings
     */
    public function update(Request $request)
    {
        $validated = $request->validate([
            'settings' => 'required|array',
            'settings.*.id' => 'required|exists:agent_settings,id',
            'settings.*.value' => 'nullable',
        ]);

        foreach ($validated['settings'] as $settingData) {
            $setting = AgentSetting::find($settingData['id']);

            if ($setting) {
                // Don't update if value is masked (for sensitive fields)
                if ($settingData['value'] !== '********') {
                    $setting->update([
                        'value' => $settingData['value'],
                    ]);
                }
            }
        }

        // Clear cache after updating
        AgentConfig::clearCache();

        return response()->json([
            'message' => 'Settings updated successfully',
        ]);
    }

    /**
     * Update a single setting
     */
    public function updateSingle(Request $request, $id)
    {
        $validated = $request->validate([
            'value' => 'nullable',
        ]);

        $setting = AgentSetting::findOrFail($id);

        // Don't update if value is masked (for sensitive fields)
        if ($validated['value'] !== '********') {
            $setting->update([
                'value' => $validated['value'],
            ]);
        }

        // Clear cache after updating
        AgentConfig::clearCache();

        return response()->json([
            'message' => 'Setting updated successfully',
            'setting' => $setting,
        ]);
    }

    /**
     * Import settings from config file
     */
    public function importFromConfig()
    {
        $count = AgentConfig::importFromConfig();

        return response()->json([
            'message' => "Successfully imported {$count} settings from config file",
            'count' => $count,
        ]);
    }

    /**
     * Clear settings cache
     */
    public function clearCache()
    {
        AgentConfig::clearCache();

        return response()->json([
            'message' => 'Cache cleared successfully',
        ]);
    }

    /**
     * Test connection with current settings
     */
    public function testConnection()
    {
        $results = [];
        $allPassed = true;

        // Test tenant connectivity
        try {
            $tenantTest = AgentConnectivityHelper::testConnectivity();
            $results[] = [
                'name' => 'ForceDesk Tenant',
                'status' => $tenantTest ? 'success' : 'failed',
                'message' => $tenantTest ? 'Successfully connected to tenant' : 'Failed to connect to tenant',
            ];
            if (!$tenantTest) {
                $allPassed = false;
            }
        } catch (\Exception $e) {
            $results[] = [
                'name' => 'ForceDesk Tenant',
                'status' => 'error',
                'message' => 'Error: ' . $e->getMessage(),
            ];
            $allPassed = false;
        }

        // Test LDAP connectivity if configured
        $ldapConfigured = agent_config('ldap.ad_dc') !== null;
        if ($ldapConfigured) {
            try {
                $ldapTest = AgentConnectivityHelper::testLdapConnectivity();
                $results[] = [
                    'name' => 'LDAP/Active Directory',
                    'status' => $ldapTest ? 'success' : 'failed',
                    'message' => $ldapTest ? 'Successfully connected to LDAP' : 'Failed to connect to LDAP',
                ];
                if (!$ldapTest) {
                    $allPassed = false;
                }
            } catch (\Exception $e) {
                $results[] = [
                    'name' => 'LDAP/Active Directory',
                    'status' => 'error',
                    'message' => 'Error: ' . $e->getMessage(),
                ];
                $allPassed = false;
            }
        }

        return response()->json([
            'success' => $allPassed,
            'message' => $allPassed ? 'All connectivity tests passed' : 'Some connectivity tests failed',
            'results' => $results,
        ]);
    }

    /**
     * Get Laravel logs
     */
    public function getLogs(Request $request)
    {
        $logPath = storage_path('logs');
        $search = $request->query('search', '');
        $lines = $request->query('lines', 500);

        // Find the most recent log file
        $logFiles = glob($logPath . '/laravel*.log');
        if (empty($logFiles)) {
            return response()->json([
                'content' => 'No log files found',
                'file' => null,
            ]);
        }

        // Sort by modified time, most recent first
        usort($logFiles, function ($a, $b) {
            return filemtime($b) - filemtime($a);
        });

        $mostRecentLog = $logFiles[0];
        $fileName = basename($mostRecentLog);

        // Read the log file
        if (!file_exists($mostRecentLog)) {
            return response()->json([
                'content' => 'Log file not found',
                'file' => $fileName,
            ]);
        }

        // Read the last N lines
        $logContent = $this->readLastLines($mostRecentLog, $lines);

        // Apply search filter if provided
        if (!empty($search)) {
            $logLines = explode("\n", $logContent);
            $filteredLines = array_filter($logLines, function ($line) use ($search) {
                return stripos($line, $search) !== false;
            });
            $logContent = implode("\n", $filteredLines);
        }

        return response()->json([
            'content' => $logContent,
            'file' => $fileName,
        ]);
    }

    /**
     * Read last N lines from a file
     */
    private function readLastLines($file, $lines)
    {
        $handle = fopen($file, 'r');
        $linecounter = $lines;
        $pos = -2;
        $beginning = false;
        $text = [];

        while ($linecounter > 0) {
            $t = ' ';
            while ($t != "\n") {
                if (fseek($handle, $pos, SEEK_END) == -1) {
                    $beginning = true;
                    break;
                }
                $t = fgetc($handle);
                $pos--;
            }
            $linecounter--;
            if ($beginning) {
                rewind($handle);
            }
            $text[$lines - $linecounter - 1] = fgets($handle);
            if ($beginning) {
                break;
            }
        }
        fclose($handle);
        return implode('', array_reverse($text));
    }

    /**
     * Download entire log file
     */
    public function downloadLogs()
    {
        $logPath = storage_path('logs');

        // Find the most recent log file
        $logFiles = glob($logPath . '/laravel*.log');
        if (empty($logFiles)) {
            return response()->json([
                'error' => 'No log files found',
            ], 404);
        }

        // Sort by modified time, most recent first
        usort($logFiles, function ($a, $b) {
            return filemtime($b) - filemtime($a);
        });

        $mostRecentLog = $logFiles[0];
        $fileName = basename($mostRecentLog);

        return response()->download($mostRecentLog, $fileName);
    }

    /**
     * Export settings to config/agentconfig.php file
     */
    public function exportConfig()
    {
        $settings = AgentSetting::all()->groupBy('group');

        $config = "<?php\n\nreturn [\n\n";

        foreach ($settings as $group => $groupSettings) {
            $config .= "    '{$group}' => [\n";

            foreach ($groupSettings as $setting) {
                // Extract the setting name from the full key (e.g., 'tenant.tenant_url' -> 'tenant_url')
                $parts = explode('.', $setting->key);
                $settingName = end($parts);

                // Add description as comment if available
                if ($setting->description) {
                    $config .= "        // {$setting->description}\n";
                }

                // Format the value based on type
                $value = $this->formatConfigValue($setting->value, $setting->type);

                $config .= "        '{$settingName}' => {$value},\n";
            }

            $config .= "    ],\n";
        }

        $config .= "];\n";

        // Write to config/agentconfig.php
        $configPath = config_path('agentconfig.php');
        file_put_contents($configPath, $config);

        return response()->json([
            'message' => 'Configuration exported successfully to config/agentconfig.php',
            'path' => $configPath,
        ]);
    }

    /**
     * Format a value for PHP config file export
     */
    private function formatConfigValue($value, $type): string
    {
        return match ($type) {
            'boolean' => $value ? 'true' : 'false',
            'integer' => (string) $value,
            'float' => (string) $value,
            'json' => var_export(json_decode($value, true), true),
            default => $value === null ? 'null' : "'" . addslashes($value) . "'",
        };
    }
}
