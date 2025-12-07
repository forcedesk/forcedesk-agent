<?php

namespace App\Console\Commands\DeviceManager;

use App\Models\CollectorPayloads;
use Illuminate\Console\Command;
use Illuminate\Support\Facades\Http;
use Illuminate\Support\Str;

class DeviceManagerQuery extends Command
{
    /**
     * The name and signature of the console command.
     *
     * @var string
     */
    protected $signature = 'devicemanager:query-collector
                            {--poll-interval=5 : Seconds between polling for payloads}
                            {--max-runtime=300 : Maximum runtime in seconds before restart}
                            {--api-url= : Override API URL from environment}
                            {--agent-uuid= : Override agent UUID from environment}';

    /**
     * The console command description.
     *
     * @var string
     */
    protected $description = 'Process Device Manager query payloads from the collector queue';

    /**
     * Execute the console command.
     */
    public function handle()
    {
        $pollInterval = 5;
        $maxRuntime = 300;

        $this->info('===========================================');
        $this->info('Device Manager Collector Processor');
        $this->info('===========================================');
        $this->info("API URL: {$apiUrl}");
        $this->info("Agent UUID: {$agentUuid}");
        $this->info("Poll Interval: {$pollInterval}s");
        $this->info("Max Runtime: {$maxRuntime}s");
        $this->info('===========================================');
        $this->newLine();

        $startTime = time();

        while (time() - $startTime < $maxRuntime) {
            try {
                $this->info('[' . now()->format('Y-m-d H:i:s') . '] Fetching payloads from API...');

                // Fetch payloads from the API
                $response = Http::withHeaders([
                    'Authorization' => 'Bearer ' . config('agentconfig.tenant.tenant_api_key'),
                    'Content-Type' => 'application/json',
                    'x-forcedesk-agent' => config('agentconfig.tenant.tenant_uuid'),
                    'x-forcedesk-agentversion' => config('app.agent_version'),
                    'Accept' => 'application/json',
                ])->get("{$apiUrl}/api/agent/devicemanager/query-payloads");

                if (!$response->successful()) {
                    $this->error("API request failed: {$response->status()}");
                    $this->line($response->body());
                    sleep($pollInterval);
                    continue;
                }

                $data = $response->json();

                if ($data['status'] !== 'success' || empty($data['payloads'])) {
                    $this->comment('No pending payloads. Waiting...');
                    sleep($pollInterval);
                    continue;
                }

                $payloads = $data['payloads'];
                $config = $data['config'] ?? [];
                $legacySshOptions = $config['legacy_ssh_options'] ?? '-o StrictHostKeyChecking=no -oKexAlgorithms=+diffie-hellman-group1-sha1';

                $this->info("Found {$data['count']} pending payload(s)");
                $this->line("Legacy SSH options: " . substr($legacySshOptions, 0, 50) . "...");

                // Process each payload
                foreach ($payloads as $payload) {
                    $this->processPayload($payload, $apiUrl, $agentUuid, $agentVersion, $legacySshOptions);
                }

            } catch (\Exception $e) {
                $this->error("ERROR: {$e->getMessage()}");
                sleep($pollInterval);
            }
        }

        $this->newLine();
        $this->info('===========================================');
        $this->info('Max runtime reached. Exiting...');
        $this->info('===========================================');

        return 0;
    }

    /**
     * Process a single payload
     */
    protected function processPayload($payload, $apiUrl, $agentUuid, $agentVersion, $legacySshOptions)
    {
        $payloadId = $payload['id'];
        $payloadData = $payload['payload_data'];

        $this->newLine();
        $this->info('[' . now()->format('Y-m-d H:i:s') . "] Processing payload ID: {$payloadId}");

        // Extract payload information
        $deviceHostname = $payloadData->device_hostname ?? null;
        $username = $payloadData->username ?? null;
        $password = $payloadData->password ?? null;
        $port = $payloadData->port ?? 22;
        $command = $payloadData->command ?? null;
        $deviceType = $payloadData->device_type ?? 'cisco';
        $action = $payloadData->action ?? 'unknown';
        $isCiscoLegacy = $payloadData->is_cisco_legacy ?? false;

        if (!$deviceHostname || !$username || !$password || !$command) {
            $this->error('Missing required payload data');
            $this->postResponse($payloadId, [
                'status' => 'error',
                'error' => 'Missing required payload data',
                'timestamp' => now()->toDateTimeString(),
            ], $apiUrl, $agentUuid, $agentVersion);
            return;
        }

        // Execute SSH command
        try {
            $this->line("  → Connecting to {$username}@{$deviceHostname}:{$port}");
            $this->line("  → Device Type: {$deviceType}" . ($isCiscoLegacy ? ' (Legacy)' : ''));
            $this->line("  → Action: {$action}");
            $this->line("  → Executing command...");

            $output = $this->executeSSHCommand($deviceHostname, $port, $username, $password, $command, $isCiscoLegacy, $legacySshOptions);

            if ($output === false) {
                throw new \Exception('SSH command execution failed');
            }

            $outputSize = strlen($output);
            $this->info("  ✓ Command executed successfully ({$outputSize} bytes)");

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

            // Post response to API
            $this->postResponse($payloadId, $responseData, $apiUrl, $agentUuid, $agentVersion);

            $this->info("  ✓ Payload processed successfully");

        } catch (\Exception $e) {
            $this->error("ERROR: {$e->getMessage()}");

            $errorData = [
                'status' => 'error',
                'error' => $e->getMessage(),
                'message' => $e->getMessage(),
                'timestamp' => now()->toDateTimeString(),
            ];

            $this->postResponse($payloadId, $errorData, $apiUrl, $agentUuid, $agentVersion);
        }
    }

    /**
     * Execute SSH command on remote device
     */
    protected function executeSSHCommand($hostname, $port, $username, $password, $command, $isLegacy = false, $legacySshOptions = '')
    {
        // Use a temporary file for storing credentials
        $passwordFile = tmpfile();
        $passwordFileUri = stream_get_meta_data($passwordFile)['uri'];
        fwrite($passwordFile, $password);

        // Determine SSH options based on whether device is legacy
        if ($isLegacy && !empty($legacySshOptions)) {
            // Use legacy SSH options from configuration
            $sshOptions = $legacySshOptions . ' -o ConnectTimeout=15 -o ServerAliveInterval=10';
            $this->line("  → Using legacy SSH options for older device");
        } else {
            // Use standard SSH options
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

        return implode("\n", $output);
    }

    /**
     * POST response to API endpoint
     */
    protected function postResponse($payloadId, $responseData, $apiUrl, $agentUuid, $agentVersion)
    {
        $this->line("  → Posting response to API...");

        try {
            $response = Http::withHeaders([
                'Authorization' => 'Bearer ' . config('agentconfig.tenant.tenant_api_key'),
                'Content-Type' => 'application/json',
                'x-forcedesk-agent' => config('agentconfig.tenant.tenant_uuid'),
                'x-forcedesk-agentversion' => config('app.agent_version'),
                'Accept' => 'application/json',
                'Content-Type' => 'application/json',
            ])->post("{$apiUrl}/api/agent/devicemanager/query-response", [
                'payload_id' => $payloadId,
                'response_data' => $responseData,
            ]);

            if ($response->successful()) {
                $this->info("  ✓ API POST successful (HTTP {$response->status()})");
            } else {
                $this->warn("  ✗ API POST returned HTTP {$response->status()}");
                $this->line("  Response: " . $response->body());
            }
        } catch (\Exception $e) {
            $this->error("  ✗ API POST failed: " . $e->getMessage());
        }
    }
}
