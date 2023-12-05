<?php

/****************************************************************************
 * SchoolDesk - The School Helpdesk System
 *
 * Copyright © 2019 - Excelion/Samuel Brereton. All Rights Reserved.
 *
 * This file or any other component of SchoolDesk cannot be copied, altered
 * and/or distributed without the express permission of SamueL Brereton.
 *
 * Your use of this software is governed by the SchoolDesk EULA. No warranty
 * is expressed or implied except otherwise laid out in your Support Agreement.
 *
 ***************************************************************************/

namespace App\Helper;

use GuzzleHttp\Client;

class AgentConnectivityHelper
{
    public static function testConnectivity(): bool
    {
        $client = new Client(['verify' => false, 'headers' => array(
            'Authorization' => 'Bearer ' . config('agentconfig.tenant.tenant_api_key'),
            'Content-Type' => 'application/json',
            'x-schooldesk-agent' => config('agentconfig.tenant.tenant_uuid'),
            'x-schooldesk-agentversion' => config('app.agent_version'),
        )]);

        $request = $client->get(config('agentconfig.tenant.tenant_url') . '/api/agent/test');

        $response = $request->getBody()->getContents();
        $data = json_decode($response, false);

        if ($data->status == 'ok')
        {
            return true;
        } else {
            return false;
        }
    }
}
