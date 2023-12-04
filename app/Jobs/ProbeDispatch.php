<?php

namespace App\Jobs;

use GuzzleHttp\Client;
use Illuminate\Bus\Queueable;
use Illuminate\Contracts\Queue\ShouldQueue;
use Illuminate\Foundation\Bus\Dispatchable;
use Illuminate\Queue\InteractsWithQueue;
use Illuminate\Queue\SerializesModels;
use Illuminate\Support\Facades\Process;

class ProbeDispatch implements ShouldQueue
{
    use Dispatchable, InteractsWithQueue, Queueable, SerializesModels;

    public $probe;

    public function __construct($probe)
    {
        $this->probe = $probe;
    }

    public function handle()
    {

        $client = new Client(['verify' => false, 'headers' => array(
            'Authorization' => 'Bearer ' . config('agentconfig.tenant.tenant_api_key'),
            'Content-Type' => 'application/json',
            'x-schooldesk-agent' => config('agentconfig.tenant.tenant_uuid'),
            'x-schooldesk-agentversion' => config('app.agent_version'),
        )]);

        if ($this->probe->check_type == 'tcp') {

            // Run the probe via the Process facade.
            $tcpProbe = Process::run('nc -vzw 5 -q 2 '.$this->probe->host.' '.$this->probe->port);

            if ($tcpProbe->successful()) {

                $data = [
                    'id' => $this->probe->probeid,
                    'status' => 'up',
                ];

                $response = $client->post(config('agentconfig.tenant.tenant_url') . '/api/agent/monitoring/response', [
                    'headers' => [],
                    'body' => json_encode($data),
                ]);

            } else {

                $data = [
                    'id' => $this->probe->probeid,
                    'status' => 'down',
                ];

                $response = $client->post(config('agentconfig.tenant.tenant_url') . '/api/agent/monitoring/response', [
                    'headers' => [],
                    'body' => json_encode($data),
                ]);

            }

        }

        if ($this->probe->check_type == 'ping') {

            $pingProbe = Process::run('fping -c5 '.$this->probe->host);

            if ($pingProbe->successful()) {

                $data = [
                    'id' => $this->probe->probeid,
                    'status' => 'up',
                ];

                $response = $client->post(config('agentconfig.tenant.tenant_url') . '/api/agent/monitoring/response', [
                    'headers' => [],
                    'body' => json_encode($data),
                ]);

            } else {

                $data = [
                    'id' => $this->probe->probeid,
                    'status' => 'down',
                ];

                $response = $client->post(config('agentconfig.tenant.tenant_url') . '/api/agent/monitoring/response', [
                    'headers' => [],
                    'body' => json_encode($data),
                ]);

            }

        }
    }
}
