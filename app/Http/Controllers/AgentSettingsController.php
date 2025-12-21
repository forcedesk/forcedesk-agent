<?php

namespace App\Http\Controllers;

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
        // You can implement connection testing logic here
        // For example, test tenant connection, LDAP connection, etc.

        return response()->json([
            'message' => 'Connection test not yet implemented',
        ]);
    }
}
