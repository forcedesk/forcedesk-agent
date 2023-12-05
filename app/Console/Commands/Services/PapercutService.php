<?php

namespace App\Console\Commands\Services;

use App\Models\Students;
use App\Models\User;
use GuzzleHttp\Client;
use Illuminate\Console\Command;
use function Laravel\Prompts\progress;

class PapercutService extends Command
{
    /**
     * The name and signature of the console command.
     *
     * @var string
     */
    protected $signature = 'agent:papercut-service';

    /**
     * The console command description.
     *
     * @var string
     */
    protected $description = 'Retrieves and sends Papercut data to tenant.';

    /**
     * Execute the console command.
     *
     * @return void
     */
    public function handle(): void
    {
        \App\Jobs\PapercutAgent::dispatch();
        $this->info('Papercut Service job dispatched...');
    }
}
