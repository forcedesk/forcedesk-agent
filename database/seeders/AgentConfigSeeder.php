<?php

namespace Database\Seeders;

use Illuminate\Database\Console\Seeds\WithoutModelEvents;
use Illuminate\Database\Seeder;
use App\Helpers\AgentConfig;

class AgentConfigSeeder extends Seeder
{
    /**
     * Run the database seeds.
     */
    public function run(): void
    {
        $count = AgentConfig::importFromConfig();
        $this->command->info("Imported {$count} settings from config file.");
    }
}
