<?php

namespace App\Console\Commands;

use App\Jobs\ProbeDispatch;
use GuzzleHttp\Client;
use Illuminate\Console\Command;

class MonitoringChecks extends Command
{
    /**
     * The name and signature of the console command.
     *
     * @var string
     */
    protected $signature = 'agent:monitoring-checks';

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

        if(config('agentconfig.schooldeskagent.strict_logging') == true) {
            \Log::info('Logging is enabled');
        }

        $client = new Client(['verify' => false, 'headers' => array(
            'Authorization' => 'Bearer ' . config('agentconfig.schooldeskagent.tenant_api_key'),
            'Content-Type' => 'application/json',
        )]);

        echo "Connecting to: " . config('agentconfig.schooldeskagent.tenant_url') . '/api/agent/payloads/monitoring';

        $request = $client->get(config('agentconfig.schooldeskagent.tenant_url') . '/api/agent/payloads/monitoring');

        $response = $request->getBody()->getContents();
        $data = json_decode($response, false);

        $itemcount = count($data);

        if($itemcount >= 1) {
            \Log::info("SchoolDesk Agent received ".$itemcount." payloads.");
        } else {
            \Log::info("SchoolDesk Agent received no payloads. Sleeping until next run....");
        }

        foreach ($data as $item) {

            /* Handle Probe Checks */
            if ($item->type == 'probecheck') {
                foreach ($item->payload_data as $payload) {
                    ProbeDispatch::dispatch($payload);
                }
            }

            /* Handle Device Backup Requests */
            if ($item->type == 'devicebackup') {
                foreach ($item->payload_data as $payload) {
                    DeviceBackup::dispatch($payload);
                }
            }

            /* Handle Password Resets */
            if ($item->type == 'passwordreset') {
                foreach ($item->payload_data as $payload) {
                    DeviceBackup::dispatch($payload);
                }
            }

            /* Handle Papercut PIN requests */
            if ($item->type == 'papercutpin') {
                foreach ($item->payload_data as $payload) {
                    DeviceBackup::dispatch($payload);
                }
            }

            /* Handle Papercut Balance Requests */
            if ($item->type == 'papercutbal') {
                foreach ($item->payload_data as $payload) {
                    DeviceBackup::dispatch($payload);
                }
            }

            /* Handle EduPass Imports */
            if ($item->type == 'edupassimport') {
                foreach ($item->payload_data as $payload) {
                    DeviceBackup::dispatch($payload);
                }
            }

        }

    }
}
