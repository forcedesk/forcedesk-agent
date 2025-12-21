<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Model;
use Illuminate\Support\Facades\Cache;

class AgentSetting extends Model
{
    protected $fillable = [
        'key',
        'value',
        'type',
        'group',
        'description',
        'is_sensitive',
    ];

    protected $casts = [
        'is_sensitive' => 'boolean',
    ];

    /**
     * Get a setting value by key with optional default fallback
     */
    public static function getValue(string $key, mixed $default = null): mixed
    {
        try {
            return Cache::remember("agent_setting_{$key}", 3600, function () use ($key, $default) {
                $setting = self::where('key', $key)->first();

                if (!$setting) {
                    return $default;
                }

                return self::castValue($setting->value, $setting->type);
            });
        } catch (\Exception $e) {
            // If cache fails, get directly from database
            $setting = self::where('key', $key)->first();
            return $setting ? self::castValue($setting->value, $setting->type) : $default;
        }
    }

    /**
     * Set a setting value
     */
    public static function setValue(string $key, mixed $value, string $type = 'string', string $group = 'general', bool $isSensitive = false): void
    {
        $setting = self::updateOrCreate(
            ['key' => $key],
            [
                'value' => $value,
                'type' => $type,
                'group' => $group,
                'is_sensitive' => $isSensitive,
            ]
        );

        try {
            Cache::forget("agent_setting_{$key}");
        } catch (\Exception $e) {
            // Cache operation failed, but setting was saved to database
        }
    }

    /**
     * Get all settings grouped by their group
     */
    public static function getAllGrouped(): array
    {
        $settings = self::all();
        $grouped = [];

        foreach ($settings as $setting) {
            if (!isset($grouped[$setting->group])) {
                $grouped[$setting->group] = [];
            }

            $grouped[$setting->group][$setting->key] = [
                'value' => self::castValue($setting->value, $setting->type),
                'type' => $setting->type,
                'is_sensitive' => $setting->is_sensitive,
                'description' => $setting->description,
            ];
        }

        return $grouped;
    }

    /**
     * Cast value to appropriate type
     */
    protected static function castValue(mixed $value, string $type): mixed
    {
        return match ($type) {
            'boolean' => filter_var($value, FILTER_VALIDATE_BOOLEAN),
            'integer' => (int) $value,
            'float' => (float) $value,
            'json' => json_decode($value, true),
            default => $value,
        };
    }

    /**
     * Clear all setting caches
     */
    public static function clearCache(): void
    {
        try {
            $settings = self::all();
            foreach ($settings as $setting) {
                Cache::forget("agent_setting_{$setting->key}");
            }
        } catch (\Exception $e) {
            // Cache clearing failed, continue anyway
        }
    }
}
