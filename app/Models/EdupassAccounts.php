<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;

class EdupassAccounts extends Model
{
    use HasFactory;

    protected $fillable = ['login', 'firstName', 'lastName', 'displayName', 'password', 'ldap_dn'];

    public function getPasswordAttribute($value)
    {
        return \Crypt::decrypt($value);
    }

    public function setPasswordAttribute($value)
    {
        $this->attributes['password'] = \Crypt::encrypt($value);
    }
}
