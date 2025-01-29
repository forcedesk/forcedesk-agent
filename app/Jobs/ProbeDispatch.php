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

        /* Generate a new Guzzle Client for handling the payload from SchoolDesk */
        $client = new Client(['verify' => false, 'headers' => array(
            'Authorization' => 'Bearer ' . config('agentconfig.tenant.tenant_api_key'),
            'Content-Type' => 'application/json',
            'x-schooldesk-agent' => config('agentconfig.tenant.tenant_uuid'),
            'x-schooldesk-agentversion' => config('app.agent_version'),
        )]);

        /* Generate Ping Data for Time-Series Graphs */
        $pingdata = $this->generateMetricData($this->probe->host);

        \Log::info($pingdata);

        /* Handle a TCP Check */
        if ($this->probe->check_type == 'tcp') {

            // Run the probe via the Process facade.
            $tcpProbe = Process::run('nc -vzw 5 -q 2 '.$this->probe->host.' '.$this->probe->port);

            if ($tcpProbe->successful()) {

                $data = [
                    'id' => $this->probe->probeid,
                    'ping_data' => $pingdata['ping'],
                    'packet_loss_data' => $pingdata['packet_loss'],
                    'status' => 'up',
                ];

                $response = $client->post(config('agentconfig.tenant.tenant_url') . '/api/agent/monitoring/response', [
                    'headers' => [],
                    'body' => json_encode($data),
                ]);

            } else {

                $data = [
                    'id' => $this->probe->probeid,
                    'ping_data' => $pingdata['ping'],
                    'packet_loss_data' => $pingdata['packet_loss'],
                    'status' => 'down',
                ];

                $response = $client->post(config('agentconfig.tenant.tenant_url') . '/api/agent/monitoring/response', [
                    'headers' => [],
                    'body' => json_encode($data),
                ]);

            }

        }

        /* Handle a Ping-Only Check */
        if ($this->probe->check_type == 'ping') {

            $pingProbe = Process::run('fping -c5 '.$this->probe->host);

            if ($pingProbe->successful()) {

                $data = [
                    'id' => $this->probe->probeid,
                    'ping_data' => $pingdata['ping'],
                    'packet_loss_data' => $pingdata['packet_loss'],
                    'status' => 'up',
                ];

                $response = $client->post(config('agentconfig.tenant.tenant_url') . '/api/agent/monitoring/response', [
                    'headers' => [],
                    'body' => json_encode($data),
                ]);

            } else {

                $data = [
                    'id' => $this->probe->probeid,
                    'ping_data' => $pingdata['ping'],
                    'packet_loss_data' => $pingdata['packet_loss'],
                    'status' => 'down',
                ];

                $response = $client->post(config('agentconfig.tenant.tenant_url') . '/api/agent/monitoring/response', [
                    'headers' => [],
                    'body' => json_encode($data),
                ]);

            }

        }
    }

    private function generateMetricData(string $host)
    {
        /* Ping the requested host and return the availability data */
        \Log::info("fping -C 5 -q $host");
        $pingresult = Process::run("fping -C 5 -q $host");

        $output = $pingresult->errorOutput();

        if (preg_match('/\d+\/\d+\/(\d+)%/', $output, $lossMatch)) {
            $packetLoss = $lossMatch[1];
        } else {
            $packetLoss = null;
        }

        if (preg_match('/\s*:\s*(.+)/', $output, $matches)) {
            $pingTimes = explode(' ', trim($matches[1]));
            $pingTimes = array_map('floatval', $pingTimes);

            if (!empty($pingTimes) && count($pingTimes) > 0) {
                $pingData = array_sum($pingTimes) / count($pingTimes);

                return [
                    'ping' => $pingData,
                    'packet_loss' => $packetLoss
                ];
            }
        }

        return null;
    }
}
