<?php

namespace App\Console\Commands;

use App\Services\PasswordResetService;
use GuzzleHttp\Client;
use Illuminate\Console\Command;

class HeartbeatService extends Command
{
    public function __construct()
    {
        parent::__construct();
    }
    /**
     * The name and signature of the console command.
     *
     * @var string
     */
    protected $signature = 'agent:heartbeat';

    /**
     * The console command description.
     *
     * @var string
     */
    protected $description = 'Run the agent and tests for connectivity between itself and the tenant.';

    /**
     * Execute the console command.
     *
     * @return int
     */
    public function handle()
    {

        $client = new Client(['verify' => false, 'headers' => array(
            'Authorization' => 'Bearer ' . config('agentconfig.tenant.tenant_api_key'),
            'Content-Type' => 'application/json',
            'x-forcedesk-agent' => config('agentconfig.tenant.tenant_uuid'),
            'x-forcedesk-agentversion' => config('app.agent_version'),
        )]);

        $request = $client->get(config('agentconfig.tenant.tenant_url') . '/api/agent/heartbeat');

        $response = $request->getBody()->getContents();
        $data = json_decode($response, false);

        if ($data->status == 'ok')
        {
            $this->info($data->message);
            return true;
        } else {

            if($data->message)
            {
                $this->error($data->message);
                return false;
            } else {
                $this->error('Heartbeat Failure.');
                return false;
            }

        }
    }
}
