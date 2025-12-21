<?php

namespace App\Console\Commands\Services;

use App\Helper\AgentConnectivityHelper;
use App\Jobs\DeviceManagerRunner;
use App\Jobs\ProbeDispatch;
use GuzzleHttp\Client;
use GuzzleHttp\Exception\GuzzleException;
use Illuminate\Console\Command;
use Illuminate\Support\Str;

class DeviceManagerService extends Command
{
    /**
     * The name and signature of the console command.
     *
     * @var string
     */
    protected $signature = 'agent:devicemanager-service';

    /**
     * The console command description.
     *
     * @var string
     */
    protected $description = 'Backs up managed devices and sends the data to the SchoolDesk Tenant.';

    public function handle()
    {

        $batchid = Str::uuid();

        $test = AgentConnectivityHelper::testConnectivity();

        if(!$test)
        {
            \Log::error('Could not connect to the SchoolDesk instance.');
            $this->error('Connectivity failed to the SchoolDesk instance. Bailing out');
            return Command::FAILURE;
        }

        $client = new Client([
            'verify' => agent_config('tenant.verify_ssl', true),
            'timeout' => 30,
            'connect_timeout' => 10,
            'headers' => array(
                'Authorization' => 'Bearer ' . agent_config('tenant.tenant_api_key'),
                'Content-Type' => 'application/json',
                'x-forcedesk-agent' => agent_config('tenant.tenant_uuid'),
                'x-forcedesk-agentversion' => config('app.agent_version'),
            )
        ]);

        try {
            $request = $client->get(agent_config('tenant.tenant_url') . '/api/agent/devicemanager/payloads');

            if ($request->getStatusCode() !== 200) {
                \Log::error('device manager service received non-200 status code', [
                    'status_code' => $request->getStatusCode()
                ]);
                $this->error('Failed to fetch device manager payloads. Status: ' . $request->getStatusCode());
                return Command::FAILURE;
            }

            $response = $request->getBody()->getContents();

            $data = json_decode($response, false);

            if (json_last_error() !== JSON_ERROR_NONE) {
                \Log::error('Invalid JSON response from device manager service', [
                    'error' => json_last_error_msg()
                ]);
                $this->error('Invalid JSON response from device manager service');
                return Command::FAILURE;
            }

            if (!is_array($data) && !is_object($data)) {
                \Log::error('Unexpected data type from device manager service', [
                    'type' => gettype($data)
                ]);
                $this->error('Unexpected response format from device manager service');
                return Command::FAILURE;
            }

            if (count($data) === 0)
            {
                \Log::info('No device manager payloads received');
                $this->info('No device manager payloads received');
                return Command::SUCCESS;
            }

            $dispatchedCount = 0;

            foreach($data as $item)
            {
                if (!isset($item->payload_data) || !is_array($item->payload_data)) {
                    \Log::warning('Skipping item with missing or invalid payload_data', [
                        'item' => $item
                    ]);
                    continue;
                }

                foreach ($item->payload_data as $payload) {
                    try {
                        DeviceManagerRunner::dispatch($payload, $batchid);
                        $dispatchedCount++;
                    } catch (\Exception $e) {
                        \Log::error('Failed to dispatch backup', [
                            'payload' => $payload,
                            'error' => $e->getMessage()
                        ]);
                    }
                }
            }

            \Log::info('Device Manager service completed successfully', [
                'payloads_dispatched' => $dispatchedCount
            ]);
            $this->info("Successfully dispatched {$dispatchedCount} device manager backup(s)");

            return Command::SUCCESS;

        } catch (GuzzleException $e) {
            \Log::error('HTTP request failed in monitoring service', [
                'error' => $e->getMessage(),
                'url' => agent_config('tenant.tenant_url') . '/api/agent/devicemanager/payloads'
            ]);
            $this->error('HTTP request failed: ' . $e->getMessage());
            return Command::FAILURE;
        } catch (\Exception $e) {
            \Log::error('Unexpected error in device manager service', [
                'error' => $e->getMessage(),
                'trace' => $e->getTraceAsString()
            ]);
            $this->error('Unexpected error: ' . $e->getMessage());
            return Command::FAILURE;
        }

    }
}
