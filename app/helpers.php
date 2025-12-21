<?php

use App\Helpers\AgentConfig;

if (!function_exists('agent_config')) {
    /**
     * Get agent configuration value from database or config file
     *
     * @param string|null $key
     * @param mixed $default
     * @return mixed
     */
    function agent_config(?string $key = null, mixed $default = null): mixed
    {
        if ($key === null) {
            return AgentConfig::getAllGrouped();
        }

        return AgentConfig::get($key, $default);
    }
}
