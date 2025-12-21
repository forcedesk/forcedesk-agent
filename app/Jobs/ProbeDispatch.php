<?php

namespace App\Jobs;

use GuzzleHttp\Client;
use GuzzleHttp\Exception\GuzzleException;
use Illuminate\Bus\Queueable;
use Illuminate\Contracts\Queue\ShouldQueue;
use Illuminate\Foundation\Bus\Dispatchable;
use Illuminate\Queue\InteractsWithQueue;
use Illuminate\Queue\SerializesModels;
use Illuminate\Support\Facades\Log;
use Illuminate\Support\Facades\Process;

class ProbeDispatch implements ShouldQueue
{
    use Dispatchable, InteractsWithQueue, Queueable, SerializesModels;

    public $probe;

    public function __construct($probe)
    {
        $this->probe = $probe;
    }

    public function handle(): void
    {
        try {
            // Validate probe object has required properties
            if (!$this->validateProbe()) {
                Log::error('Invalid probe object received', [
                    'probe' => $this->probe
                ]);
                $this->fail(new \Exception('Invalid probe object'));
                return;
            }

            /* Generate a new Guzzle Client for handling the payload from SchoolDesk */
            $client = new Client([
                'verify' => agent_config('tenant.verify_ssl', true),
                'timeout' => 30,
                'connect_timeout' => 10,
                'headers' => [
                    'Authorization' => 'Bearer ' . agent_config('tenant.tenant_api_key'),
                    'Content-Type' => 'application/json',
                    'x-forcedesk-agent' => agent_config('tenant.tenant_uuid'),
                    'x-forcedesk-agentversion' => config('app.agent_version'),
                ]
            ]);

            /* Generate Ping Data for Time-Series Graphs */
            $pingdata = $this->generateMetricData($this->probe->host);

            if ($pingdata === null) {
                Log::warning('Failed to generate metric data for probe', [
                    'probe_id' => $this->probe->probeid ?? 'unknown',
                    'host' => $this->probe->host
                ]);
                // Continue with null values rather than failing completely
                $pingdata = ['ping' => null, 'packet_loss' => null];
            }

            Log::info('Probe metric data generated', [
                'probe_id' => $this->probe->probeid,
                'host' => $this->probe->host,
                'ping' => $pingdata['ping'],
                'packet_loss' => $pingdata['packet_loss']
            ]);

            $status = null;

            /* Handle a TCP Check */
            if ($this->probe->check_type === 'tcp') {
                $status = $this->performTcpCheck($this->probe->host, $this->probe->port);
            }

            /* Handle a Ping-Only Check */
            if ($this->probe->check_type === 'ping') {
                $status = $this->performPingCheck($this->probe->host);
            }

            if ($status === null) {
                Log::error('Unknown or unsupported check type', [
                    'probe_id' => $this->probe->probeid,
                    'check_type' => $this->probe->check_type
                ]);
                $this->fail(new \Exception('Unknown check type: ' . $this->probe->check_type));
                return;
            }

            // Send results back to tenant
            $this->sendProbeResults($client, $this->probe->probeid, $pingdata, $status);

        } catch (\Exception $e) {
            Log::error('Probe dispatch job failed', [
                'probe_id' => $this->probe->probeid ?? 'unknown',
                'error' => $e->getMessage(),
                'trace' => $e->getTraceAsString()
            ]);
            $this->fail($e);
        }
    }

    private function validateProbe(): bool
    {
        if (!is_object($this->probe)) {
            return false;
        }

        $requiredFields = ['check_type', 'host', 'probeid'];

        foreach ($requiredFields as $field) {
            if (!isset($this->probe->$field)) {
                return false;
            }
        }

        // Validate check type
        if (!in_array($this->probe->check_type, ['tcp', 'ping'])) {
            return false;
        }

        // TCP checks require port
        if ($this->probe->check_type === 'tcp' && !isset($this->probe->port)) {
            return false;
        }

        return true;
    }

    private function performTcpCheck(string $host, $port): string
    {
        // Sanitize inputs to prevent command injection
        $safeHost = escapeshellarg($host);
        $safePort = escapeshellarg($port);

        $tcpProbe = Process::run("nc -vzw 5 -q 2 $safeHost $safePort");

        $status = $tcpProbe->successful() ? 'up' : 'down';

        Log::info('TCP probe completed', [
            'host' => $host,
            'port' => $port,
            'status' => $status,
            'exit_code' => $tcpProbe->exitCode()
        ]);

        return $status;
    }

    private function performPingCheck(string $host): string
    {
        // Sanitize input to prevent command injection
        $safeHost = escapeshellarg($host);

        $pingProbe = Process::run("fping -c5 $safeHost");

        $status = $pingProbe->successful() ? 'up' : 'down';

        Log::info('Ping probe completed', [
            'host' => $host,
            'status' => $status,
            'exit_code' => $pingProbe->exitCode()
        ]);

        return $status;
    }

    private function sendProbeResults(Client $client, $probeId, array $pingdata, string $status): void
    {
        $data = [
            'id' => $probeId,
            'ping_data' => $pingdata['ping'],
            'packet_loss_data' => $pingdata['packet_loss'],
            'status' => $status,
        ];

        try {
            $response = $client->post(agent_config('tenant.tenant_url') . '/api/agent/monitoring/response', [
                'headers' => [],
                'body' => json_encode($data),
            ]);

            if ($response->getStatusCode() !== 200 && $response->getStatusCode() !== 201) {
                Log::error('Failed to send probe results - non-success status', [
                    'probe_id' => $probeId,
                    'status_code' => $response->getStatusCode(),
                    'response_body' => $response->getBody()->getContents()
                ]);
                throw new \Exception('Non-success status code: ' . $response->getStatusCode());
            }

            Log::info('Probe results sent successfully', [
                'probe_id' => $probeId,
                'status' => $status,
                'http_status' => $response->getStatusCode()
            ]);

        } catch (GuzzleException $e) {
            Log::error('Failed to send probe results - HTTP error', [
                'probe_id' => $probeId,
                'error' => $e->getMessage()
            ]);
            throw $e;
        }
    }

    private function generateMetricData(string $host): ?array
    {
        // Sanitize input to prevent command injection
        $safeHost = escapeshellarg($host);

        $ping_process = Process::run("fping -C 5 -q $safeHost");
        $packet_loss_process = Process::run("fping -c 5 -q $safeHost");

        $ping_result = $ping_process->errorOutput();
        $packet_loss_result = $packet_loss_process->errorOutput();

        if (preg_match('/\d+\/\d+\/(\d+)%/', $packet_loss_result, $lossMatch)) {
            $packetLoss = $lossMatch[1];
        } else {
            $packetLoss = null;
        }

        if (preg_match('/\s*:\s*(.+)/', $ping_result, $matches)) {
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
