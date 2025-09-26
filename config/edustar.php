<?php

return [

    /*
    |--------------------------------------------------------------------------
    | eduSTAR Management Console Configuration
    |--------------------------------------------------------------------------
    |
    | This file contains the configuration settings for connecting to the
    | eduSTAR Management Console API.
    |
    */

    /*
    |--------------------------------------------------------------------------
    | Authentication Credentials
    |--------------------------------------------------------------------------
    |
    | These credentials are used to authenticate with the eduSTAR MC API.
    | You can set these in your .env file using EDUSTAR_USERNAME and
    | EDUSTAR_PASSWORD variables.
    |
    */

    'username' => env('EDUSTAR_USERNAME'),
    'password' => env('EDUSTAR_PASSWORD'),

    /*
    |--------------------------------------------------------------------------
    | API Configuration
    |--------------------------------------------------------------------------
    |
    | Configuration settings for the eduSTAR Management Console API.
    |
    */

    'base_url' => env('EDUSTAR_BASE_URL', 'https://apps.edustar.vic.edu.au/edustarmc/'),
    'api_url' => env('EDUSTAR_API_URL', 'https://apps.edustar.vic.edu.au/edustarmc/api/'),
    'login_url' => env('EDUSTAR_LOGIN_URL', 'https://apps.edustar.vic.edu.au/my.policy'),

    /*
    |--------------------------------------------------------------------------
    | Connection Settings
    |--------------------------------------------------------------------------
    |
    | Settings that control how the service connects to eduSTAR MC.
    |
    */

    'max_attempts' => env('EDUSTAR_MAX_ATTEMPTS', 3),
    'retry_delay' => env('EDUSTAR_RETRY_DELAY', 10), // seconds
    'timeout' => env('EDUSTAR_TIMEOUT', 30), // seconds
    'session_cleanup_delay' => env('EDUSTAR_SESSION_CLEANUP_DELAY', 5), // seconds

    /*
    |--------------------------------------------------------------------------
    | Default Headers
    |--------------------------------------------------------------------------
    |
    | Default HTTP headers sent with each request to eduSTAR MC.
    |
    */

    'headers' => [
        'user_agent' => env('EDUSTAR_USER_AGENT', 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36'),
        'accept' => 'application/json, text/plain, */*',
        'accept_language' => 'en-US,en;q=0.9',
    ],

    /*
    |--------------------------------------------------------------------------
    | Logging
    |--------------------------------------------------------------------------
    |
    | Control logging behavior for eduSTAR MC operations.
    |
    */

    'logging' => [
        'enabled' => env('EDUSTAR_LOGGING_ENABLED', true),
        'level' => env('EDUSTAR_LOGGING_LEVEL', 'info'),
        'channel' => env('EDUSTAR_LOGGING_CHANNEL', 'default'),
    ],

];
