<?php

namespace App\Console\Commands\Services;

use App\Services\PasswordResetService;
use GuzzleHttp\Client;
use Illuminate\Console\Command;

class CommandQueueService extends Command
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
    protected $signature = 'agent:process-command-queue';

    /**
     * The console command description.
     *
     * @var string
     */
    protected $description = 'Run the agent and check for any command queues.';

    /**
     * Execute the console command.
     *
     * @return int
     */
    public function handle(): void
    {

        $client = new Client(['verify' => false, 'headers' => array(
            'Authorization' => 'Bearer ' . config('agentconfig.tenant.tenant_api_key'),
            'Content-Type' => 'application/json',
            'x-schooldesk-agent' => config('agentconfig.tenant.tenant_uuid'),
            'x-schooldesk-agentversion' => config('app.agent_version'),
        )]);

        $request = $client->get(config('agentconfig.tenant.tenant_url') . '/api/agent/command-queues');

        $response = $request->getBody()->getContents();
        $data = json_decode($response, false);

        foreach($data as $item)
        {
            if($item->type == 'force-sync-papercutsvc' && $item->payload_data->process)
            {
                \App\Jobs\PapercutAgent::dispatch();
            }
        }
    }
}
