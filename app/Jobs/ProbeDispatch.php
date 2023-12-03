<?php

namespace App\Jobs;

use GuzzleHttp\Client;
use Illuminate\Bus\Queueable;
use Illuminate\Contracts\Queue\ShouldQueue;
use Illuminate\Foundation\Bus\Dispatchable;
use Illuminate\Queue\InteractsWithQueue;
use Illuminate\Queue\SerializesModels;
use PhpProbe\Probe\PingProbe;
use PhpProbe\Probe\TcpProbe;

class ProbeDispatch implements ShouldQueue
{
    use Dispatchable, InteractsWithQueue, Queueable, SerializesModels;

    /**
     * The podcast instance.
     *
     * @var \App\Models\MonitoringProbes
     */
    public $probe;

    /**
     * Create a new job instance.
     *
     * @return void
     */
    public function __construct($probe)
    {
        $this->probe = $probe;
    }

    /**
     * Execute the job.
     *
     * @return void
     */
    public function handle()
    {

        $client = new Client(['verify' => false, 'headers' => array(
            'Authorization' => 'Bearer ' . config('agentconfig.schooldeskagent.tenant_api_key'),
            'Content-Type' => 'application/json',
        )]);

        $checkprobe = $this->probe;

        if (config('agentconfig.schooldeskagent.strict_logging') == true) {
            \Log::info('Checking a probe...');
        }

        if ($checkprobe->check_type == 'tcp') {

            $tcpProbe = new TcpProbe($checkprobe->name, array(), new \PhpProbe\Adapter\NetcatAdapter());
            $tcpProbe->host($checkprobe->host)->port($checkprobe->port);

            try {
                $tcpProbe->check();
            } catch (Symfony\Component\Process\Exception\ProcessTimedOutException$e) {
                \Log::info('An exception occurred');
            }

            if ($tcpProbe->hasSucceeded()) {

                $data = [
                    'responsetype' => 'probecheck',
                    'probeid' => $checkprobe->probeid,
                    'probestatus' => 'up',
                ];

                if (config('agentconfig.schooldeskagent.strict_logging') == true) {
                    \Log::info('Probe was up...');
                }

                $response = $client->post(config('agentconfig.schooldeskagent.tenant_url') . '/api/agent/response', [
                    'headers' => [],
                    'body' => json_encode($data),
                ]);

            } else {

                $data = [
                    'responsetype' => 'probecheck',
                    'probeid' => $checkprobe->probeid,
                    'probestatus' => 'down',
                ];

                if (config('agentconfig.schooldeskagent.strict_logging') == true) {
                    \Log::info('Probe was down...');
                }

                $response = $client->post(config('agentconfig.schooldeskagent.tenant_url') . '/api/agent/response', [
                    'headers' => [],
                    'body' => json_encode($data),
                ]);

            }

        }

        if ($checkprobe->check_type == 'ping') {

            $pingProbe = new PingProbe($checkprobe->name, array(), new \PhpProbe\Adapter\PingAdapter());
            $pingProbe->host($checkprobe->host);

            try {
                $pingProbe->check();
            } catch (RuntimeException $exception) {
                \Log::info('An exception occurred');
            }

            if ($pingProbe->hasSucceeded()) {

                $data = [
                    'responsetype' => 'probecheck',
                    'probeid' => $checkprobe->probeid,
                    'probestatus' => 'up',
                ];

                if (config('agentconfig.schooldeskagent.strict_logging') == true) {
                    \Log::info('Probe was up...');
                }

                $response = $client->post(config('agentconfig.schooldeskagent.tenant_url') . '/api/agent/response', [
                    'headers' => [],
                    'body' => json_encode($data),
                ]);

            } else {

                $data = [
                    'responsetype' => 'probecheck',
                    'probeid' => $checkprobe->probeid,
                    'probestatus' => 'down',
                ];

                if (config('agentconfig.schooldeskagent.strict_logging') == true) {
                    \Log::info('Probe was down...');
                }

                $response = $client->post(config('agentconfig.schooldeskagent.tenant_url') . '/api/agent/response', [
                    'headers' => [],
                    'body' => json_encode($data),
                ]);

            }

        }
    }
}
