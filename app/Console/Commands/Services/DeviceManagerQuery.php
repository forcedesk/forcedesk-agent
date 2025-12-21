<?php

namespace App\Console\Commands\Services;

use App\Jobs\ProcessDeviceQuery;
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
    protected $signature = 'devicemanager:collector';

    /**
     * The console command description.
     *
     * @var string
     */
    protected $description = 'Process Device Manager query payloads from the collector queue (runs for 300 seconds)';

    /**
     * Execute the console command.
     */
    public function handle()
    {
        $pollInterval = 15; // Check every 2 seconds
        $maxRuntime = 300; // Run for 300 seconds (5 minutes)

        $startTime = time();

        while (time() - $startTime < $maxRuntime) {
            try {
                $this->info('[' . now()->format('Y-m-d H:i:s') . '] Fetching payloads from API...');

                // Fetch payloads from the API
                $response = Http::withHeaders([
                    'Authorization' => 'Bearer ' . agent_config('tenant.tenant_api_key'),
                    'Content-Type' => 'application/json',
                    'x-forcedesk-agent' => agent_config('tenant.tenant_uuid'),
                    'x-forcedesk-agentversion' => config('app.agent_version'),
                    'Accept' => 'application/json',
                ])->get(agent_config('tenant.tenant_url') . "/api/agent/devicemanager/query-payloads");

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

                // Dispatch each payload as a job for parallel processing
                foreach ($payloads as $payload) {
                    ProcessDeviceQuery::dispatch($payload, $legacySshOptions);
                    $this->comment("  ✓ Dispatched job for payload ID: {$payload['id']}");
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
}
