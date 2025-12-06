<?php

namespace App\Console\Commands\Services;

use App\Helper\AgentConnectivityHelper;
use App\Jobs\ProbeDispatch;
use GuzzleHttp\Client;
use GuzzleHttp\Exception\GuzzleException;
use Illuminate\Console\Command;

class MonitoringService extends Command
{
    /**
     * The name and signature of the console command.
     *
     * @var string
     */
    protected $signature = 'agent:monitoring-service';

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

        $test = AgentConnectivityHelper::testConnectivity();

        if(!$test)
        {
            \Log::error('Could not connect to the SchoolDesk instance.');
            $this->error('Connectivity failed to the SchoolDesk instance. Bailing out');
            return Command::FAILURE;
        }

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

        try {
            $request = $client->get(config('agentconfig.tenant.tenant_url') . '/api/agent/monitoring/getpayloads');

            if ($request->getStatusCode() !== 200) {
                \Log::error('Monitoring service received non-200 status code', [
                    'status_code' => $request->getStatusCode()
                ]);
                $this->error('Failed to fetch monitoring payloads. Status: ' . $request->getStatusCode());
                return Command::FAILURE;
            }

            $response = $request->getBody()->getContents();

            $data = json_decode($response, false);

            if (json_last_error() !== JSON_ERROR_NONE) {
                \Log::error('Invalid JSON response from monitoring service', [
                    'error' => json_last_error_msg()
                ]);
                $this->error('Invalid JSON response from monitoring service');
                return Command::FAILURE;
            }

            if (!is_array($data) && !is_object($data)) {
                \Log::error('Unexpected data type from monitoring service', [
                    'type' => gettype($data)
                ]);
                $this->error('Unexpected response format from monitoring service');
                return Command::FAILURE;
            }

            if (count($data) === 0)
            {
                \Log::info('No monitoring payloads received');
                $this->info('No monitoring payloads received');
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
                        ProbeDispatch::dispatch($payload);
                        $dispatchedCount++;
                    } catch (\Exception $e) {
                        \Log::error('Failed to dispatch probe', [
                            'payload' => $payload,
                            'error' => $e->getMessage()
                        ]);
                    }
                }
            }

            \Log::info('Monitoring service completed successfully', [
                'payloads_dispatched' => $dispatchedCount
            ]);
            $this->info("Successfully dispatched {$dispatchedCount} monitoring probe(s)");

            return Command::SUCCESS;

        } catch (GuzzleException $e) {
            \Log::error('HTTP request failed in monitoring service', [
                'error' => $e->getMessage(),
                'url' => config('agentconfig.tenant.tenant_url') . '/api/agent/monitoring/getpayloads'
            ]);
            $this->error('HTTP request failed: ' . $e->getMessage());
            return Command::FAILURE;
        } catch (\Exception $e) {
            \Log::error('Unexpected error in monitoring service', [
                'error' => $e->getMessage(),
                'trace' => $e->getTraceAsString()
            ]);
            $this->error('Unexpected error: ' . $e->getMessage());
            return Command::FAILURE;
        }

    }
}
