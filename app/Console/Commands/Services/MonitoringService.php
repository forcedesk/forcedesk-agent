<?php

namespace App\Console\Commands\Services;

use App\Helper\AgentConnectivityHelper;
use App\Jobs\ProbeDispatch;
use GuzzleHttp\Client;
use Illuminate\Console\Command;

class MonitoringService extends Command
{
    /**
     * The name and signature of the console command.
     *
     * @var string
     */
    protected $signature = 'agent:monitoring-service';

    /**
     * The console command description.
     *
     * @var string
     */
    protected $description = 'Runs the agent service to check for monitoring checks from the tenant.';

    /**
     * Execute the console command.
     *
     * @return int
     */
    public function handle()
    {

        $test = AgentConnectivityHelper::testConnectivity();

        if(!$test)
        {
            \Log::error('Could not connect to the SchoolDesk instance.');
            $this->error('Connectivity failed to the SchoolDesk instance. Bailing out');
            return false;
        }

        $client = new Client(['verify' => false, 'headers' => array(
            'Authorization' => 'Bearer ' . config('agentconfig.tenant.tenant_api_key'),
            'Content-Type' => 'application/json',
            'x-forcedesk-agent' => config('agentconfig.tenant.tenant_uuid'),
            'x-forcedesk-agentversion' => config('app.agent_version'),
        )]);

        $request = $client->get(config('agentconfig.tenant.tenant_url') . '/api/agent/monitoring/getpayloads');

        $response = $request->getBody()->getContents();

        $data = json_decode($response, false);

        if (count($data) == '0')
        {
            $this->error('No monitoring payloads received');
            return false;
        }

        foreach($data as $item)
        {
            foreach ($item->payload_data as $payload) {
                ProbeDispatch::dispatch($payload);
            }
        }

        return true;

    }
}
