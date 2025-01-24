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

        /* Handle a TCP Check */
        if ($this->probe->check_type == 'tcp') {

            // Run the probe via the Process facade.
            $tcpProbe = Process::run('nc -vzw 5 -q 2 '.$this->probe->host.' '.$this->probe->port);

            if ($tcpProbe->successful()) {

                $data = [
                    'id' => $this->probe->probeid,
                    'ping_data' => $pingdata,
                    'status' => 'up',
                ];

                $response = $client->post(config('agentconfig.tenant.tenant_url') . '/api/agent/monitoring/response', [
                    'headers' => [],
                    'body' => json_encode($data),
                ]);

            } else {

                $data = [
                    'id' => $this->probe->probeid,
                    'ping_data' => $pingdata,
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
                    'ping_data' => $pingdata,
                    'status' => 'up',
                ];

                $response = $client->post(config('agentconfig.tenant.tenant_url') . '/api/agent/monitoring/response', [
                    'headers' => [],
                    'body' => json_encode($data),
                ]);

            } else {

                $data = [
                    'id' => $this->probe->probeid,
                    'ping_data' => $pingdata,
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
        $pingresult = Process::run("fping -C 5 -q $host");

        $output = $pingresult->output(); // This captures stdout
        $errorOutput = $pingresult->errorOutput(); // This captures stderr

        \Log::info(['stdout' => $output, 'stderr' => $errorOutput]);

        /* Check if the result has failed otherwise format the ping times using the fping regex */
        if ($pingresult->failed()) {
            return null;
        } else {
            $output = $pingresult->output(); // Should be something like: "8.8.8.8 : 15.12 12.34 10.78 13.45 14.01"
            preg_match('/: (.+)/', $output, $matches);
        }

        /* If the data doesn't match the regex, return no data */
        if (!isset($matches[1])) {
            return null;
        }

        /* Format the ping times into a singular rounded average. */
        $pingTimes = explode(' ', trim($matches[1]));
        $pingdata = (int) round(array_sum($pingTimes) / count($pingTimes));

        /* Return the ping time back to the handler */
        return $pingdata;
    }
}
