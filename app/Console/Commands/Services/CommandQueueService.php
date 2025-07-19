<?php

namespace App\Console\Commands\Services;

use App\Helper\AgentConnectivityHelper;
use App\Models\Students;
use App\Models\User;
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

        $request = $client->get(config('agentconfig.tenant.tenant_url') . '/api/agent/command-queues');

        $response = $request->getBody()->getContents();
        $data = json_decode($response, false);

        foreach($data as $item)
        {

            if($item->type == 'force-sync-edustarsvc' && $item->payload_data->process)
            {
                /* Fire the console command */
                \Artisan::call('agent:edustar-service');
            }

            if($item->type == 'force-sync-papercutsvc' && $item->payload_data->process)
            {
                /* Fire the console command */
                \Artisan::call('agent:papercut-service');
            }
        }

        return true;
    }
}
