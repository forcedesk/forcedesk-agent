<?php

namespace App\Helper;

use GuzzleHttp\Client;

class AgentConnectivityHelper
{
    /**
     * Tests connectivity to the SchoolDesk instance
     * before processing any commands.
     *
     * @return bool
     * @throws \GuzzleHttp\Exception\GuzzleException
     */
    public static function testConnectivity(): bool
    {

        $client = new Client(['verify' => false, 'headers' => array(
            'Authorization' => 'Bearer ' . agent_config('tenant.tenant_api_key'),
            'Content-Type' => 'application/json',
            'x-forcedesk-agent' => agent_config('tenant.tenant_uuid'),
            'x-forcedesk-agentversion' => config('app.agent_version'),
        )]);

        $request = $client->get(agent_config('tenant.tenant_url') . '/api/agent/test');

        $response = $request->getBody()->getContents();
        $data = json_decode($response, false);

        if ($data->status == 'ok')
        {
            return true;
        } else {
            return false;
        }
    }

    public static function testLdapConnectivity(): bool
    {

        $test = \Artisan::call('ldap:test');

        if($test)
        {
            return true;
        } else {
            return false;
        }

    }
}
