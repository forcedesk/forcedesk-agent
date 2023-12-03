<?php

return [

    /*
    |--------------------------------------------------------------------------
    | Default LDAP Connection Name
    |--------------------------------------------------------------------------
    |
    | Here you may specify which of the LDAP connections below you wish
    | to use as your default connection for all LDAP operations. Of
    | course you may add as many connections you'd like below.
    |
    */

    'default' => env('LDAP_CONNECTION', 'default'),

    /*
    |--------------------------------------------------------------------------
    | LDAP Connections
    |--------------------------------------------------------------------------
    |
    | Below you may configure each LDAP connection your application requires
    | access to. Be sure to include a valid base DN - otherwise you may
    | not receive any results when performing LDAP search operations.
    |
    */

    'connections' => [

        'default' => [
            'hosts' => [config('agentconfig.ldap.ad_dc')],
            'username' => env('LDAP_USERNAME', config('agentconfig.ldap.ad_svc_user_cn')),
            'password' => env('LDAP_PASSWORD', config('agentconfig.ldap.ad_svc_password')),
            'port' => env('LDAP_PORT', 636),
            'base_dn' => env('LDAP_BASE_DN', config('agentconfig.ldap.ad_base_dn')),
            'timeout' => env('LDAP_TIMEOUT', 5),
            'use_ssl' => env('LDAP_SSL', true),
            'use_tls' => env('LDAP_TLS', false),
        ],

        'ldap' => [
            'hosts' => [config('agentconfig.ldap.ad_dc')],
            'username' => env('LDAP_USERNAME', config('agentconfig.ldap.ad_svc_user_cn')),
            'password' => env('LDAP_PASSWORD', config('agentconfig.ldap.ad_svc_password')),
            'port' => env('LDAP_PORT', 636),
            'base_dn' => env('LDAP_BASE_DN', config('agentconfig.ldap.ad_base_dn')),
            'timeout' => env('LDAP_TIMEOUT', 5),
            'use_ssl' => env('LDAP_SSL', true),
            'use_tls' => env('LDAP_TLS', false),
        ],

    ],

    /*
    |--------------------------------------------------------------------------
    | LDAP Logging
    |--------------------------------------------------------------------------
    |
    | When LDAP logging is enabled, all LDAP search and authentication
    | operations are logged using the default application logging
    | driver. This can assist in debugging issues and more.
    |
    */

    'logging' => env('LDAP_LOGGING', false),

    /*
    |--------------------------------------------------------------------------
    | LDAP Cache
    |--------------------------------------------------------------------------
    |
    | LDAP caching enables the ability of caching search results using the
    | query builder. This is great for running expensive operations that
    | may take many seconds to complete, such as a pagination request.
    |
    */

    'cache' => [
        'enabled' => env('LDAP_CACHE', false),
        'driver' => env('CACHE_DRIVER', 'file'),
    ],

];
