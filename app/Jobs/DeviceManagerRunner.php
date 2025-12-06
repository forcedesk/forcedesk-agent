<?php

namespace App\Jobs;

use App\Models\DeviceManagerBackups;
use App\Models\DeviceManagerDevices;
use App\Models\DeviceManagerLogs;
use GuzzleHttp\Client;
use GuzzleHttp\Exception\GuzzleException;
use Illuminate\Bus\Queueable;
use Illuminate\Contracts\Queue\ShouldBeEncrypted;
use Illuminate\Contracts\Queue\ShouldQueue;
use Illuminate\Foundation\Bus\Dispatchable;
use Illuminate\Queue\InteractsWithQueue;
use Illuminate\Queue\SerializesModels;
use Illuminate\Support\Facades\Process;
use Illuminate\Support\Str;
use Log;

class DeviceManagerRunner implements ShouldBeEncrypted, ShouldQueue
{
    use Dispatchable, InteractsWithQueue, Queueable, SerializesModels;

    public $device;
    public $batchid;

    public function __construct($device, $batchid)
    {
        $this->device = $device;
        $this->batchid = $batchid;
    }

    public function handle(): void
    {

        $device = $this->device;

        if ($device->device_username && $device->device_password) {
            $process = $this->runBackupCommand($device);

            if ($process && $process->successful() && strlen($process->output()) >= 10) {
                $configData = $this->parseConfigData($device, $process->output());
                $this->sendBackup($device, $configData);
            } else {
                $this->logFailedBackup($device, $process->output());
            }
        }
    }

    protected function runBackupCommand($device): \Illuminate\Process\ProcessResult|\Illuminate\Contracts\Process\ProcessResult|null
    {
        $passwordFile = tmpfile();
        fwrite($passwordFile, $device->device_password);
        $passwordFileUri = stream_get_meta_data($passwordFile)['uri'];

        $command = match ($device->type) {
            'cisco' => $this->getCiscoCommand($device, $passwordFileUri),
            'mikrotik' => "sshpass -f $passwordFileUri ssh -p {$device->port} -o StrictHostKeyChecking=no -oKexAlgorithms=+diffie-hellman-group1-sha1 {$device->device_username}@{$device->hostname} 'export show-sensitive verbose'",
            default => null
        };

        $process = $command ? Process::run($command) : null;

        fwrite($passwordFile, Str::random(4096));
        fclose($passwordFile);

        return $process;
    }

    protected function getCiscoCommand($device, $passwordFileUri): string
    {
        $baseCommand = "sshpass -f $passwordFileUri ssh -p {$device->port} ";
        $legacyCommand = config('applicationconfig.device_manager.legacycommand');
        $options = $device->is_cisco_legacy ? "$legacyCommand {$device->device_username}@{$device->hostname} 'show running-config view full'"
            : "-o StrictHostKeyChecking=no -oKexAlgorithms=+diffie-hellman-group1-sha1 {$device->device_username}@{$device->hostname} 'show running-config view full'";

        return $baseCommand.$options;
    }

    protected function parseConfigData($device, $output): false|string
    {
        return match ($device->type) {
            'cisco' => strstr(strstr($output, '!'), 'version'),
            'mikrotik' => strstr($output, '/interface'),
            default => $output
        };
    }

    protected function sendBackup($device, $configData): void
    {
        $latestBackup = $device->latesthash;

        $backupDataHash = hash('sha256', $configData);

        $client = new Client([
            'verify' => config('agentconfig.tenant.verify_ssl', true),
            'timeout' => 30,
            'connect_timeout' => 10,
            'headers' => array(
                'Authorization' => 'Bearer ' . config('agentconfig.tenant.tenant_api_key'),
                'Content-Type' => 'application/json',
                'x-forcedesk-agent' => config('agentconfig.tenant.tenant_uuid'),
                'x-forcedesk-agentversion' => config('app.agent_version'),
            )
        ]);

        if ($latestBackup !== $backupDataHash) {
            $payload = [
                'device_id' => $device->id,
                'data' => $configData,
                'uuid' => Str::uuid(),
                'batch' => $this->batchid,
                'size' => strlen($configData),
                'status' => 'success',
                'log' => "Backup for Device: {$device->name} was successful."
            ];

            try {
                $response = $client->post(config('agentconfig.tenant.tenant_url') . '/api/agent/devicemanager/response', [
                    'headers' => [],
                    'body' => json_encode($payload),
                ]);

                if ($response->getStatusCode() !== 200 && $response->getStatusCode() !== 201) {
                    \Illuminate\Support\Facades\Log::error('Failed to send device manager backup - non-success status', [
                        'status_code' => $response->getStatusCode(),
                        'response_body' => $response->getBody()->getContents()
                    ]);
                    throw new \Exception('Non-success status code: ' . $response->getStatusCode());
                }

                Log::info('Device manager backup sent successfully', [
                    'http_status' => $response->getStatusCode()
                ]);

            } catch (GuzzleException $e) {
                Log::error('Failed to send device manager backup - HTTP error', [
                    'error' => $e->getMessage()
                ]);
                throw $e;
            }

        } else {
            $payload = [
                'device_id' => $device->id,
                'status' => 'failed',
                'log' => "Backup for Device: {$device->name} failed."
            ];

            try {
                $response = $client->post(config('agentconfig.tenant.tenant_url') . '/api/agent/devicemanager/response', [
                    'headers' => [],
                    'body' => json_encode($payload),
                ]);

                if ($response->getStatusCode() !== 200 && $response->getStatusCode() !== 201) {
                    \Illuminate\Support\Facades\Log::error('Failed to send device manager backup - non-success status', [
                        'status_code' => $response->getStatusCode(),
                        'response_body' => $response->getBody()->getContents()
                    ]);
                    throw new \Exception('Non-success status code: ' . $response->getStatusCode());
                }

                Log::info('Device manager backup sent successfully', [
                    'http_status' => $response->getStatusCode()
                ]);

            } catch (GuzzleException $e) {
                Log::error('Failed to send device manager backup - HTTP error', [
                    'error' => $e->getMessage()
                ]);
                throw $e;
            }
        }
    }

    private function logFailedBackup($device, string $output)
    {
        Log::error('Failed to backup device', [
            'error' => $output
        ]);
    }
}
