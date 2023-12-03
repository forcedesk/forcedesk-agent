<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    /**
     * Run the migrations.
     */
    public function up(): void
    {
        Schema::create('edupass_accounts', function (Blueprint $table) {
            $table->id();
            $table->string('login');
            $table->string('ldap_dn')->nullable();
            $table->string('password');
            $table->string('firstName');
            $table->string('lastName');
            $table->string('displayName');
            $table->integer('year')->nullable();
            $table->string('student_class')->nullable();
            $table->timestamps();
        });
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        Schema::dropIfExists('edupass_accounts');
    }
};
