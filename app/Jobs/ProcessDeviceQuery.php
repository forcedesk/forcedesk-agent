<?php

namespace App\Jobs;

use Illuminate\Bus\Queueable;
use Illuminate\Contracts\Queue\ShouldQueue;
use Illuminate\Foundation\Bus\Dispatchable;
use Illuminate\Queue\InteractsWithQueue;
use Illuminate\Queue\SerializesModels;
use Illuminate\Support\Facades\Http;
use Illuminate\Support\Facades\Log;

class ProcessDeviceQuery implements ShouldQueue
{
    use Dispatchable, InteractsWithQueue, Queueable, SerializesModels;

    /**
     * The number of times the job may be attempted.
     *
     * @var int
     */
    public $tries = 2;

    /**
     * The number of seconds the job can run before timing out.
     *
     * @var int
     */
    public $timeout = 120;

    protected $payload;
    protected $legacySshOptions;

    /**
     * Create a new job instance.
     */
    public function __construct($payload, $legacySshOptions)
    {
        $this->payload = $payload;
        $this->legacySshOptions = $legacySshOptions;
    }

    /**
     * Execute the job.
     */
    public function handle(): void
    {
        $payloadId = $this->payload['id'];
        $payloadData = $this->payload['payload_data'];

        // Ensure payload_data is an array (decode if it's a JSON string or object)
        if (is_string($payloadData)) {
            $payloadData = json_decode($payloadData, true);
        } elseif (is_object($payloadData)) {
            $payloadData = (array) $payloadData;
        }

        Log::info("Processing device query payload", ['payload_id' => $payloadId]);

        // Extract payload information from array
        $deviceHostname = $payloadData['device_hostname'] ?? null;
        $username = $payloadData['username'] ?? null;
        $password = $payloadData['password'] ?? null;
        $port = $payloadData['port'] ?? 22;
        $command = $payloadData['command'] ?? null;
        $deviceType = $payloadData['device_type'] ?? 'cisco';
        $action = $payloadData['action'] ?? 'unknown';
        $isCiscoLegacy = $payloadData['is_cisco_legacy'] ?? false;

        if (!$deviceHostname || !$username || !$password || !$command) {
            Log::error("Missing required payload data", [
                'payload_id' => $payloadId,
                'device_hostname' => $deviceHostname,
                'username' => $username,
                'has_password' => !empty($password),
                'command' => $command,
                'payload_data' => $payloadData,
            ]);
            $this->postResponse($payloadId, [
                'status' => 'error',
                'error' => 'Missing required payload data',
                'timestamp' => now()->toDateTimeString(),
            ]);
            return;
        }

        try {
            Log::info("Executing SSH command", [
                'payload_id' => $payloadId,
                'device' => $deviceHostname,
                'action' => $action,
                'legacy' => $isCiscoLegacy,
            ]);

            $output = $this->executeSSHCommand($deviceHostname, $port, $username, $password, $command, $isCiscoLegacy);

            if ($output === false) {
                throw new \Exception('SSH command execution failed');
            }

            $outputSize = strlen($output);
            Log::info("SSH command executed successfully", [
                'payload_id' => $payloadId,
                'output_size' => $outputSize,
            ]);

            // Prepare response data
            $responseData = [
                'status' => 'success',
                'output' => $output,
                'data' => $output,
                'timestamp' => now()->toDateTimeString(),
                'device_hostname' => $deviceHostname,
                'action' => $action,
                'output_size' => $outputSize,
            ];

            $this->postResponse($payloadId, $responseData);

        } catch (\Exception $e) {
            Log::error("Failed to process device query", [
                'payload_id' => $payloadId,
                'error' => $e->getMessage(),
            ]);

            $errorData = [
                'status' => 'error',
                'error' => $e->getMessage(),
                'message' => $e->getMessage(),
                'timestamp' => now()->toDateTimeString(),
            ];

            $this->postResponse($payloadId, $errorData);

            // Re-throw to trigger retry if within attempt limit
            if ($this->attempts() < $this->tries) {
                throw $e;
            }
        }
    }

    /**
     * Execute SSH command on remote device
     */
    protected function executeSSHCommand($hostname, $port, $username, $password, $command, $isLegacy = false)
    {
        // Use a temporary file for storing credentials
        $passwordFile = tmpfile();
        $passwordFileUri = stream_get_meta_data($passwordFile)['uri'];
        fwrite($passwordFile, $password);

        // Determine SSH options based on whether device is legacy
        if ($isLegacy && !empty($this->legacySshOptions)) {
            $sshOptions = $this->legacySshOptions . ' -o ConnectTimeout=15 -o ServerAliveInterval=10';
        } else {
            $sshOptions = '-o StrictHostKeyChecking=no -o ConnectTimeout=15 -o ServerAliveInterval=10 -oKexAlgorithms=+diffie-hellman-group1-sha1';
        }

        // Build SSH command
        $sshCommand = sprintf(
            "sshpass -f %s ssh -p %s %s %s@%s '%s' 2>&1",
            escapeshellarg($passwordFileUri),
            escapeshellarg($port),
            $sshOptions,
            escapeshellarg($username),
            escapeshellarg($hostname),
            $command
        );

        // Execute command
        $output = [];
        $returnVar = 0;
        exec($sshCommand, $output, $returnVar);

        // Clean up password file securely
        fseek($passwordFile, 0);
        fwrite($passwordFile, str_repeat('0', 4096));
        fclose($passwordFile);

        if ($returnVar !== 0 && empty($output)) {
            return false;
        }

        // Remove "Connection to X.X.X.X closed by remote host." text from output
        $output = array_map(function($line) {
            return preg_replace('/Connection to \d+\.\d+\.\d+\.\d+ closed( by remote host)?\.?/i', '', $line);
        }, $output);

        return implode("\n", $output);
    }

    /**
     * POST response to API endpoint
     */
    protected function postResponse($payloadId, $responseData)
    {
        try {
            $response = Http::withHeaders([
                'Authorization' => 'Bearer ' . config('agentconfig.tenant.tenant_api_key'),
                'Content-Type' => 'application/json',
                'x-forcedesk-agent' => config('agentconfig.tenant.tenant_uuid'),
                'x-forcedesk-agentversion' => config('app.agent_version'),
                'Accept' => 'application/json',
            ])->post(config('agentconfig.tenant.tenant_url') . "/api/agent/devicemanager/query-response", [
                'payload_id' => $payloadId,
                'response_data' => $responseData,
            ]);

            if ($response->successful()) {
                Log::info("API POST successful", [
                    'payload_id' => $payloadId,
                    'status' => $response->status(),
                ]);
            } else {
                Log::warning("API POST returned non-success status", [
                    'payload_id' => $payloadId,
                    'status' => $response->status(),
                    'body' => $response->body(),
                ]);
            }
        } catch (\Exception $e) {
            Log::error("API POST failed", [
                'payload_id' => $payloadId,
                'error' => $e->getMessage(),
            ]);
        }
    }

    /**
     * Handle a job failure.
     */
    public function failed(\Throwable $exception): void
    {
        Log::error("Device query job failed permanently", [
            'payload_id' => $this->payload['id'],
            'error' => $exception->getMessage(),
            'trace' => $exception->getTraceAsString(),
        ]);

        // Post failure response
        $this->postResponse($this->payload['id'], [
            'status' => 'error',
            'error' => 'Job failed after ' . $this->tries . ' attempts: ' . $exception->getMessage(),
            'timestamp' => now()->toDateTimeString(),
        ]);
    }
}
